package meter

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/billing"
	"github.com/heracraft/repose/internal/db"
)

// Rollup turns samples into usage_hours.
type Rollup struct {
	pool   *db.Pool
	pusher billing.UsagePusher
	m      *metrics.M
	log    *slog.Logger
	Now    func() time.Time
}

// NewRollup builds a rollup.
func NewRollup(pool *db.Pool, pusher billing.UsagePusher, m *metrics.M, log *slog.Logger) *Rollup {
	if pusher == nil {
		pusher = billing.Disabled{}
	}
	return &Rollup{pool: pool, pusher: pusher, m: m, log: log.With("component", "api"), Now: time.Now}
}

// Row is one usage_hours row as computed.
type Row struct {
	ProjectID      uuid.UUID
	UserID         uuid.UUID
	Class          string
	RunningSeconds int
	GBAlloc        int64
	EgressBytes    int64
	Gap            bool
	billing.Result
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
		return nil, err
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
	prows, err := r.pool.Query(ctx, `select id, user_id, class, state, volume_bytes from projects where created_at < $1 and (destroyed_at is null or destroyed_at > $2) and user_id <> '00000000-0000-7000-8000-000000000000'`, end, start)
	if err != nil {
		return nil, err
	}
	type proj struct {
		user   uuid.UUID
		class  string
		state  string
		volume int64
	}
	projects := map[uuid.UUID]proj{}
	for prows.Next() {
		var id uuid.UUID
		var p proj
		if err := prows.Scan(&id, &p.user, &p.class, &p.state, &p.volume); err != nil {
			prows.Close()
			return nil, err
		}
		projects[id] = p
	}
	prows.Close()
	var out []Row
	for pid, p := range projects {
		a := aggs[pid]
		row := Row{ProjectID: pid, UserID: p.user, Class: p.class, GBAlloc: (p.volume + (1<<30 - 1)) >> 30}
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
			r.log.Warn("no samples for a running project", "event", "billing_gap", "project_id", pid.String(), "hour", start.Format(time.RFC3339))
		}
		// Month-to-date before this hour, for the cap and the egress allowance.
		monthStart := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
		var guestCents, egressBytes, remainder int64
		err := r.pool.QueryRow(ctx, `select coalesce(sum(guest_cents),0), coalesce(sum(egress_bytes),0),
			coalesce((select storage_remainder from usage_hours where project_id = $1 and hour < $3 order by hour desc limit 1), 0)
			from usage_hours where project_id = $1 and hour >= $2 and hour < $3`, pid, monthStart, start).Scan(&guestCents, &egressBytes, &remainder)
		if err != nil {
			return out, err
		}
		periodHours := int(monthStart.AddDate(0, 1, 0).Sub(monthStart).Hours())
		row.Result = billing.Price(billing.Inputs{Class: row.Class, RunningSeconds: row.RunningSeconds, GBAlloc: row.GBAlloc, EgressBytes: row.EgressBytes,
			PeriodHours: periodHours, MonthGuestCents: guestCents, MonthEgressBytes: egressBytes, StorageRemainder: remainder})
		if _, err := r.pool.Exec(ctx, `insert into usage_hours (project_id, hour, class, running_seconds, gb_alloc, egress_bytes, cost_cents, guest_cents, storage_cents, egress_cents, storage_remainder, gap)
			values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			on conflict (project_id, hour) do update set class = excluded.class, running_seconds = excluded.running_seconds, gb_alloc = excluded.gb_alloc, egress_bytes = excluded.egress_bytes,
			cost_cents = excluded.cost_cents, guest_cents = excluded.guest_cents, storage_cents = excluded.storage_cents, egress_cents = excluded.egress_cents, storage_remainder = excluded.storage_remainder, gap = excluded.gap`,
			pid, start, row.Class, row.RunningSeconds, row.GBAlloc, row.EgressBytes, row.CostCents, row.GuestCents, row.StorageCents, row.EgressCents, row.StorageRemainder, row.Gap); err != nil {
			return out, err
		}
		out = append(out, row)
	}
	// Push rows without a Stripe record (this hour and older retries).
	if err := r.push(ctx); err != nil {
		return out, err
	}
	r.m.RollupDuration.Observe(r.Now().Sub(began).Seconds())
	r.m.RollupLagSeconds.Set(r.Now().Sub(end).Seconds())
	r.log.Info("rollup done", "event", "rollup_done", "hour", start.Format(time.RFC3339), "rows", len(out))
	return out, nil
}

func (r *Rollup) push(ctx context.Context) error {
	rows, err := r.pool.Query(ctx, `select u.project_id, p.user_id, u.hour, u.cost_cents from usage_hours u join projects p on p.id = u.project_id where u.stripe_usage_record_id is null and u.cost_cents > 0 and u.updated_at < now() order by u.hour limit 500`)
	if err != nil {
		return err
	}
	type pending struct {
		project, user uuid.UUID
		hour          time.Time
		cents         int64
	}
	var ps []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.project, &p.user, &p.hour, &p.cents); err != nil {
			rows.Close()
			return err
		}
		ps = append(ps, p)
	}
	rows.Close()
	for _, p := range ps {
		id, err := r.pusher.PushUsage(ctx, billing.UsageRow{ProjectID: p.project.String(), UserID: p.user.String(), Hour: p.hour.UTC().Format(time.RFC3339), CostCents: p.cents})
		if errors.Is(err, billing.ErrDisabled) {
			return nil
		}
		if err != nil {
			r.m.StripeUsagePushTotal.WithLabelValues("error").Inc()
			r.log.Warn("usage push failed", "event", "stripe_push_fail", "project_id", p.project.String(), "err", err.Error())
			continue
		}
		r.m.StripeUsagePushTotal.WithLabelValues("ok").Inc()
		if _, err := r.pool.Exec(ctx, "update usage_hours set stripe_usage_record_id = $3 where project_id = $1 and hour = $2", p.project, p.hour, id); err != nil {
			return err
		}
	}
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
			return 0, nil
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
	return n, nil
}
