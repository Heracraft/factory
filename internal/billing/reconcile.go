package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/obs"
)

// Reconciliation (09-billing.md §5.7): for each user, sum
// usage_hours.cost_cents minus credit rows and compare with the sum of the
// quantities Stripe recorded for the period. Any difference over 0 cents
// is a billing_mismatch alert with the project and hour; the job never
// fixes it silently.

// Mismatch is one user whose ledger and Stripe do not agree.
type Mismatch struct {
	UserID     uuid.UUID
	Handle     string
	Period     Period
	OursCents  int64
	TheirCents int64
	// Unpushed is the number of usage_hours rows in the period that still
	// owe Stripe a record, which is the usual innocent explanation.
	Unpushed int
	// FirstHour names the earliest hour of the period, so a support ticket
	// has somewhere to start with `explain`.
	FirstProject uuid.UUID
	FirstHour    time.Time
}

// Diff is the signed difference: ours minus Stripe's.
func (m Mismatch) Diff() int64 { return m.OursCents - m.TheirCents }

// Reconciler compares usage_hours with Stripe.
type Reconciler struct {
	pool   *db.Pool
	reader Reader
	m      *metrics.M
	log    *slog.Logger
	Now    func() time.Time
}

// NewReconciler builds the job. A nil reader (Stripe not configured, or no
// meter ids) makes Reconcile report that the comparison could not be made
// rather than a mismatch against zero.
func NewReconciler(pool *db.Pool, reader Reader, m *metrics.M, log *slog.Logger) *Reconciler {
	return &Reconciler{pool: pool, reader: reader, m: m, log: log.With("component", obs.ComponentAPI), Now: time.Now}
}

// ErrNoReader means Stripe is not readable, so nothing was compared.
var ErrNoReader = errors.New("no Stripe reader: set STRIPE_SECRET_KEY and the STRIPE_METER_ID_* variables to reconcile")

// Reconcile compares every non-exempt user's period. at picks the period:
// the period containing it, per user anchor.
func (r *Reconciler) Reconcile(ctx context.Context, at time.Time) ([]Mismatch, error) {
	if r.reader == nil {
		return nil, ErrNoReader
	}
	rows, err := r.pool.Query(ctx, `select id, handle, coalesce(stripe_customer_id, ''), coalesce(billing_anchor, created_at)
		from users where billing_status not in ('exempt') and stripe_customer_id is not null
		and handle <> 'repose-platform' order by handle`)
	if err != nil {
		return nil, fmt.Errorf("list users to reconcile: %w", err)
	}
	type acct struct {
		id       uuid.UUID
		handle   string
		customer string
		anchor   time.Time
	}
	var accts []acct
	for rows.Next() {
		var a acct
		if err := rows.Scan(&a.id, &a.handle, &a.customer, &a.anchor); err != nil {
			rows.Close()
			return nil, err
		}
		accts = append(accts, a)
	}
	rows.Close()
	var out []Mismatch
	var worst int64
	for _, a := range accts {
		p := PeriodFor(a.anchor, at)
		var ours int64
		var unpushed int
		var firstProject *uuid.UUID
		var firstHour *time.Time
		err := r.pool.QueryRow(ctx, `select coalesce(sum(cost_cents - credit_cents), 0),
			count(*) filter (where stripe_usage_record_id is null and cost_cents > credit_cents),
			(array_agg(project_id order by hour))[1], min(hour)
			from usage_hours u join projects pr on pr.id = u.project_id
			where pr.user_id = $1 and u.hour >= $2 and u.hour < $3`, a.id, p.Start, p.End).
			Scan(&ours, &unpushed, &firstProject, &firstHour)
		if err != nil {
			return out, fmt.Errorf("sum usage_hours for %s: %w", a.handle, err)
		}
		theirs, err := r.reader.PeriodSummary(ctx, a.customer, p.Start, p.End)
		if errors.Is(err, ErrDisabled) {
			return out, ErrNoReader
		}
		if err != nil {
			return out, err
		}
		if ours == theirs.Total() {
			continue
		}
		m := Mismatch{UserID: a.id, Handle: a.handle, Period: p, OursCents: ours, TheirCents: theirs.Total(), Unpushed: unpushed}
		if firstProject != nil {
			m.FirstProject = *firstProject
		}
		if firstHour != nil {
			m.FirstHour = firstHour.UTC()
		}
		out = append(out, m)
		if d := abs(m.Diff()); d > worst {
			worst = d
		}
		r.log.Error("usage_hours and Stripe disagree", "event", obs.EventBillingMismatch, "user_id", a.id.String(),
			"period_start", p.Start.Format(time.RFC3339), "ours_cents", ours, "stripe_cents", theirs.Total(), "unpushed_rows", unpushed)
	}
	r.m.BillingMismatchCents.Set(float64(worst))
	return out, nil
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
