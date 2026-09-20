package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/obs"
)

// The hourly rollup (09-billing.md §5.4): a job at :05 past each hour,
// idempotent, keyed on (project_id, hour). It reads meter_samples and
// nothing upstream of it, writes usage_hours, debits the trial credit in
// the same transaction, and pushes the remainder to Stripe.

// Rollup turns samples into usage_hours and usage_hours into Stripe usage.
type Rollup struct {
	pool   *db.Pool
	pusher UsagePusher
	m      *metrics.M
	log    *slog.Logger
	Now    func() time.Time
}

// NewRollup builds a rollup. A nil pusher means Stripe is not configured
// (DECISIONS I-16): rows are still written, and are pushed by a later run
// once it is.
func NewRollup(pool *db.Pool, pusher UsagePusher, m *metrics.M, log *slog.Logger) *Rollup {
	if pusher == nil {
		pusher = Disabled{}
	}
	return &Rollup{pool: pool, pusher: pusher, m: m, log: log.With("component", obs.ComponentAPI), Now: time.Now}
}

// Row is one usage_hours row as computed.
type Row struct {
	ProjectID      uuid.UUID
	UserID         uuid.UUID
	Hour           time.Time
	Class          string
	CapClass       string
	RunningSeconds int
	GBAlloc        int64
	EgressBytes    int64
	Gap            bool
	Period         Period
	CreditCents    int64
	Result
}

// Billable is what reaches Stripe for this hour.
func (r Row) Billable() int64 { return r.CostCents - r.CreditCents }

// project is what the rollup needs to know about a project for one hour.
type rollupProject struct {
	id     uuid.UUID
	user   uuid.UUID
	class  string
	state  string
	volume int64
	anchor time.Time
}

// Hour rolls up one hour [hour, hour+1h), idempotently, and returns the
// rows written.
func (r *Rollup) Hour(ctx context.Context, hour time.Time) ([]Row, error) {
	start := hour.UTC().Truncate(time.Hour)
	end := start.Add(time.Hour)
	began := r.Now()
	type agg struct {
		running   int
		class     string
		diskAlloc int64
		egress    int64
		samples   int
	}
	aggs := map[uuid.UUID]*agg{}
	rows, err := r.pool.Query(ctx, `select project_id, count(*) filter (where state = 'running'), count(*), max(disk_alloc), sum(net_tx),
		(array_agg(class order by ts desc))[1] from meter_samples where ts >= $1 and ts < $2 group by project_id`, start, end)
	if err != nil {
		return nil, fmt.Errorf("read meter_samples for %s: %w", start.Format(time.RFC3339), err)
	}
	for rows.Next() {
		var pid uuid.UUID
		var a agg
		var egress *int64
		if err := rows.Scan(&pid, &a.running, &a.samples, &a.diskAlloc, &egress, &a.class); err != nil {
			rows.Close()
			return nil, err
		}
		if egress != nil {
			a.egress = *egress
		}
		aggs[pid] = &a
	}
	rows.Close()
	// Projects that existed during the hour accrue storage even without
	// samples; a running project without samples is a gap.
	projects, err := r.projectsFor(ctx, start, end)
	if err != nil {
		return nil, err
	}
	var out []Row
	for _, p := range projects {
		a := aggs[p.id]
		row := Row{ProjectID: p.id, UserID: p.user, Hour: start, Class: p.class, GBAlloc: (p.volume + (1<<30 - 1)) >> 30, Period: PeriodFor(p.anchor, start)}
		if a != nil {
			row.RunningSeconds = a.running * 60
			if row.RunningSeconds > 3600 {
				row.RunningSeconds = 3600
			}
			if a.class != "" {
				row.Class = a.class
			}
			if a.diskAlloc > 0 {
				row.GBAlloc = (a.diskAlloc + (1<<30 - 1)) >> 30
			}
			row.EgressBytes = a.egress
		} else if p.state == "running" {
			row.Gap = true
			r.m.BillingGapMinutes.Add(60)
			r.log.Warn("no samples for a running project", "event", obs.EventBillingGap, "project_id", p.id.String(), "hour", start.Format(time.RFC3339))
		}
		if err := r.priceAndWrite(ctx, &row); err != nil {
			return out, err
		}
		out = append(out, row)
	}
	// Push rows without a Stripe record (this hour and older retries).
	if err := r.Push(ctx); err != nil {
		return out, err
	}
	r.m.RollupDuration.Observe(r.Now().Sub(began).Seconds())
	r.m.RollupLagSeconds.Set(r.Now().Sub(end).Seconds())
	r.log.Info("rollup done", "event", obs.EventRollupDone, "hour", start.Format(time.RFC3339), "rows", len(out))
	return out, nil
}

// projectsFor lists the projects that existed during the hour, with the
// owner's billing anchor and Stripe state.
func (r *Rollup) projectsFor(ctx context.Context, start, end time.Time) ([]rollupProject, error) {
	rows, err := r.pool.Query(ctx, `select p.id, p.user_id, p.class, p.state, p.volume_bytes,
		coalesce(u.billing_anchor, u.created_at)
		from projects p join users u on u.id = p.user_id
		where p.created_at < $1 and (p.destroyed_at is null or p.destroyed_at > $2)
		and p.user_id <> '00000000-0000-7000-8000-000000000000' order by p.id`, end, start)
	if err != nil {
		return nil, fmt.Errorf("list projects for the hour: %w", err)
	}
	defer rows.Close()
	var out []rollupProject
	for rows.Next() {
		var p rollupProject
		if err := rows.Scan(&p.id, &p.user, &p.class, &p.state, &p.volume, &p.anchor); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// periodState is the running total a project has reached in its billing
// period before the hour being priced.
type periodState struct {
	guestCents  int64
	egressBytes int64
	remainder   int64
	capClass    string
}

// periodStateFor reads the running totals for the period the hour falls in.
// The storage remainder starts at zero in a period's first hour, so a
// period sums exactly to gb_alloc * 10 rather than inheriting the previous
// period's fraction.
func (r *Rollup) periodStateFor(ctx context.Context, q store.Querier, projectID uuid.UUID, p Period, hour time.Time) (periodState, error) {
	var s periodState
	var capClass *string
	err := q.QueryRow(ctx, `select coalesce(sum(guest_cents), 0), coalesce(sum(egress_bytes), 0),
		coalesce((select storage_remainder from usage_hours where project_id = $1 and period_start = $2 and hour < $4 order by hour desc limit 1), 0),
		(select class from usage_hours where project_id = $1 and period_start = $2 and hour < $4
		 order by case class when 'xl' then 3 when 'large' then 2 when 'small' then 1 else 0 end desc limit 1)
		from usage_hours where project_id = $1 and period_start = $2 and hour >= $3 and hour < $4`,
		projectID, p.Start, p.Start, hour).Scan(&s.guestCents, &s.egressBytes, &s.remainder, &capClass)
	if err != nil {
		return s, fmt.Errorf("read the period running totals: %w", err)
	}
	if capClass != nil {
		s.capClass = *capClass
	}
	return s, nil
}

// priceAndWrite prices one hour and writes it, debiting the trial credit
// in the same transaction as the insert (§6, the trial-balance race).
func (r *Rollup) priceAndWrite(ctx context.Context, row *Row) error {
	return db.InTx(ctx, r.pool, func(tx db.Tx) error {
		// The running totals are for the period so far, up to but not
		// including this hour, so a re-run prices the hour the same way.
		st, err := r.periodStateFor(ctx, tx, row.ProjectID, row.Period, row.Hour)
		if err != nil {
			return err
		}
		row.CapClass = LargerClass(st.capClass, row.Class)
		row.Result = Price(Inputs{
			Class: row.Class, RunningSeconds: row.RunningSeconds, GBAlloc: row.GBAlloc, EgressBytes: row.EgressBytes,
			CapClass: row.CapClass, PeriodHours: row.Period.Hours(), MonthGuestCents: st.guestCents,
			MonthEgressBytes: st.egressBytes, StorageRemainder: st.remainder,
		})
		debited, err := DebitUsage(ctx, tx, row.UserID, row.ProjectID, row.Hour, row.CostCents)
		if err != nil {
			return err
		}
		row.CreditCents = debited
		_, err = tx.Exec(ctx, `insert into usage_hours (project_id, hour, class, running_seconds, gb_alloc, egress_bytes, cost_cents,
			guest_cents, storage_cents, egress_cents, storage_remainder, gap, period_start, period_end, credit_cents, price_version)
			values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
			on conflict (project_id, hour) do update set class = excluded.class, running_seconds = excluded.running_seconds,
			gb_alloc = excluded.gb_alloc, egress_bytes = excluded.egress_bytes, cost_cents = excluded.cost_cents,
			guest_cents = excluded.guest_cents, storage_cents = excluded.storage_cents, egress_cents = excluded.egress_cents,
			storage_remainder = excluded.storage_remainder, gap = excluded.gap, period_start = excluded.period_start,
			period_end = excluded.period_end, credit_cents = excluded.credit_cents, price_version = excluded.price_version`,
			row.ProjectID, row.Hour, row.Class, row.RunningSeconds, row.GBAlloc, row.EgressBytes, row.CostCents,
			row.GuestCents, row.StorageCents, row.EgressCents, row.StorageRemainder, row.Gap,
			row.Period.Start, row.Period.End, row.CreditCents, PriceVersion)
		if err != nil {
			return fmt.Errorf("write usage_hours: %w", err)
		}
		return nil
	})
}

// Push sends every usage_hours row that has no Stripe record yet, oldest
// first, and records the backlog for the stripe_push_backlog alert (§6).
func (r *Rollup) Push(ctx context.Context) error {
	type pending struct {
		project, user                       uuid.UUID
		customer                            string
		hour                                time.Time
		cost, credit, guest, storage, egres int64
		exempt                              bool
	}
	rows, err := r.pool.Query(ctx, `select u.project_id, p.user_id, coalesce(us.stripe_customer_id, ''), u.hour,
		u.cost_cents, u.credit_cents, u.guest_cents, u.storage_cents, u.egress_cents, us.billing_status = 'exempt'
		from usage_hours u join projects p on p.id = u.project_id join users us on us.id = p.user_id
		where u.stripe_usage_record_id is null and u.cost_cents > u.credit_cents and u.updated_at < now()
		order by u.hour limit 500`)
	if err != nil {
		return fmt.Errorf("list rows pending a Stripe push: %w", err)
	}
	var ps []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.project, &p.user, &p.customer, &p.hour, &p.cost, &p.credit, &p.guest, &p.storage, &p.egres, &p.exempt); err != nil {
			rows.Close()
			return err
		}
		ps = append(ps, p)
	}
	rows.Close()
	for _, p := range ps {
		if p.exempt {
			// An exempt account accrues usage_hours so the meters are
			// exercised and is never pushed to Stripe (DECISIONS I-16).
			if _, err := r.pool.Exec(ctx, "update usage_hours set stripe_usage_record_id = 'exempt' where project_id = $1 and hour = $2", p.project, p.hour); err != nil {
				return err
			}
			continue
		}
		guest, storage, egress := applyCredit(p.guest, p.storage, p.egres, p.credit)
		id, err := r.pusher.PushUsage(ctx, UsageRow{
			ProjectID: p.project.String(), UserID: p.user.String(), CustomerID: p.customer, Hour: p.hour.UTC(),
			GuestCents: guest, StorageCents: storage, EgressCents: egress, CostCents: p.cost, CreditCents: p.credit,
		})
		if errors.Is(err, ErrDisabled) {
			break
		}
		if err != nil {
			r.m.StripeUsagePushTotal.WithLabelValues("error").Inc()
			r.log.Warn("usage push failed", "event", obs.EventStripePushFail, "project_id", p.project.String(), "hour", p.hour.UTC().Format(time.RFC3339), "err", err.Error())
			continue
		}
		r.m.StripeUsagePushTotal.WithLabelValues("ok").Inc()
		if _, err := r.pool.Exec(ctx, "update usage_hours set stripe_usage_record_id = $3 where project_id = $1 and hour = $2", p.project, p.hour, id); err != nil {
			return err
		}
	}
	return r.observeBacklog(ctx)
}

// applyCredit takes the hour's credit off the three parts, cheapest line
// last, so the parts pushed to Stripe sum to the billable remainder.
func applyCredit(guest, storage, egress, credit int64) (int64, int64, int64) {
	take := func(part *int64) {
		if credit <= 0 {
			return
		}
		d := credit
		if d > *part {
			d = *part
		}
		*part -= d
		credit -= d
	}
	take(&guest)
	take(&storage)
	take(&egress)
	return guest, storage, egress
}

// observeBacklog records the oldest unpushed row's age in seconds, which is
// what the stripe_push_backlog alert reads (§6: any row older than 6 hours).
func (r *Rollup) observeBacklog(ctx context.Context) error {
	var oldest *time.Time
	if err := r.pool.QueryRow(ctx, "select min(hour) from usage_hours where stripe_usage_record_id is null and cost_cents > credit_cents").Scan(&oldest); err != nil {
		return fmt.Errorf("read the Stripe push backlog: %w", err)
	}
	if oldest == nil {
		r.m.StripePushBacklogSeconds.Set(0)
		return nil
	}
	r.m.StripePushBacklogSeconds.Set(r.Now().UTC().Sub(oldest.UTC()).Seconds())
	return nil
}

// Due rolls up every hour from the last rolled hour to the previous full
// hour; the first run starts at the earliest sample.
func (r *Rollup) Due(ctx context.Context) (int, error) {
	now := r.Now().UTC()
	last := now.Truncate(time.Hour) // exclusive end
	var from *time.Time
	if err := r.pool.QueryRow(ctx, "select max(hour) from usage_hours").Scan(&from); err != nil {
		return 0, err
	}
	var start time.Time
	if from != nil {
		start = from.UTC().Add(time.Hour)
	} else {
		var first *time.Time
		if err := r.pool.QueryRow(ctx, "select min(ts) from meter_samples").Scan(&first); err != nil {
			return 0, err
		}
		if first == nil {
			// Nothing sampled yet, but rows may still owe Stripe a push.
			return 0, r.Push(ctx)
		}
		start = first.UTC().Truncate(time.Hour)
	}
	n := 0
	for h := start; h.Before(last); h = h.Add(time.Hour) {
		if _, err := r.Hour(ctx, h); err != nil {
			return n, err
		}
		n++
	}
	if n == 0 {
		return 0, r.Push(ctx)
	}
	return n, nil
}
