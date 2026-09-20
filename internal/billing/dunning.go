package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/ops"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/obs"
)

// Past-due handling (09-billing.md §5.6): a job every hour stops every
// running guest of a user whose past_due_since is older than three days
// (snapshot first, reason billing), sends the billing_stopped
// notification, and sets billing_status = suspended. Nothing is destroyed;
// R4-11's 30-day retention starts at suspension.

// Stopper enqueues the stop of a project's guest. The ops engine satisfies
// it; a test can substitute a recorder.
type Stopper interface {
	Enqueue(ctx context.Context, q store.Querier, n ops.NewOp, allowQueue bool) (uuid.UUID, error)
	Kick()
}

// EventSink records a platform event so the notification outbox delivers
// it. events.Ingest.Platform satisfies it.
type EventSink interface {
	Platform(ctx context.Context, projectID uuid.UUID, kind, summary string) error
}

// Dunning stops and suspends past-due accounts.
type Dunning struct {
	pool   *db.Pool
	stop   Stopper
	events EventSink
	log    *slog.Logger
	// Grace is the number of days past due before guests stop; three, per
	// §5.6 and PRICING.md ("guests stopped on day 3").
	Grace time.Duration
	// Enforce is BILLING_ENFORCE (§8): false keeps the job running and
	// logging but stops nothing.
	Enforce bool
	Now     func() time.Time
}

// NewDunning builds the job.
func NewDunning(pool *db.Pool, stop Stopper, events EventSink, log *slog.Logger, enforce bool) *Dunning {
	return &Dunning{pool: pool, stop: stop, events: events, log: log.With("component", obs.ComponentAPI),
		Grace: pastDueGraceDays * 24 * time.Hour, Enforce: enforce, Now: time.Now}
}

// Stopped is one account the run acted on.
type Stopped struct {
	UserID   uuid.UUID
	Handle   string
	Projects []uuid.UUID
	OpIDs    []uuid.UUID
}

// Run stops the guests of every account past the grace period and
// suspends it. It is idempotent: an account already suspended is skipped,
// and a project with an open op is left for the next run.
func (d *Dunning) Run(ctx context.Context) ([]Stopped, error) {
	cutoff := d.Now().UTC().Add(-d.Grace)
	rows, err := d.pool.Query(ctx, `select id, handle from users
		where billing_status = 'past_due' and past_due_since is not null and past_due_since < $1
		order by past_due_since`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("list past-due accounts: %w", err)
	}
	type acct struct {
		id     uuid.UUID
		handle string
	}
	var accts []acct
	for rows.Next() {
		var a acct
		if err := rows.Scan(&a.id, &a.handle); err != nil {
			rows.Close()
			return nil, err
		}
		accts = append(accts, a)
	}
	rows.Close()
	var out []Stopped
	for _, a := range accts {
		s, err := d.suspend(ctx, a.id, a.handle)
		if err != nil {
			return out, err
		}
		out = append(out, s)
	}
	if len(out) > 0 {
		d.stop.Kick()
	}
	return out, nil
}

// suspend stops one account's guests and marks it suspended.
func (d *Dunning) suspend(ctx context.Context, userID uuid.UUID, handle string) (Stopped, error) {
	res := Stopped{UserID: userID, Handle: handle}
	projects, err := store.ListUserProjects(ctx, d.pool, userID)
	if err != nil {
		return res, err
	}
	if !d.Enforce {
		d.log.Warn("BILLING_ENFORCE=false: past-due account not stopped", "event", obs.EventBillingStopped,
			"user_id", userID.String(), "enforced", false, "projects", len(projects))
		return res, nil
	}
	for i := range projects {
		p := &projects[i]
		if p.State != "running" && p.State != "starting" {
			continue
		}
		pid := p.ID
		var opID uuid.UUID
		err := db.InTx(ctx, d.pool, func(tx db.Tx) error {
			id, err := d.stop.Enqueue(ctx, tx, ops.NewOp{
				Kind: ops.KindStop, ProjectID: &pid, Phases: ops.PlanStop(),
				Params: map[string]any{"snapshot": true, "reason": "billing"},
			}, false)
			if err != nil {
				return err
			}
			opID = id
			return nil
		})
		if errors.Is(err, ops.ErrOpInProgress) {
			// Something else is already acting on this project; the next
			// hourly run picks it up.
			continue
		}
		if err != nil {
			return res, fmt.Errorf("stop project %s for non-payment: %w", pid, err)
		}
		res.Projects = append(res.Projects, pid)
		res.OpIDs = append(res.OpIDs, opID)
		if d.events != nil {
			if err := d.events.Platform(ctx, pid, obs.EventBillingStopped,
				"Your guest was stopped because a payment failed. Update your card to start it again; nothing is deleted for 30 days."); err != nil {
				return res, err
			}
		}
	}
	if _, err := d.pool.Exec(ctx, `update users set billing_status = 'suspended', suspended_at = coalesce(suspended_at, now()),
		suspended_reason = 'billing' where id = $1 and billing_status = 'past_due'`, userID); err != nil {
		return res, err
	}
	if _, err := store.Audit(ctx, d.pool, "billing", "billing_suspend", handle, map[string]any{"projects": len(res.Projects)}); err != nil {
		return res, err
	}
	d.log.Warn("account suspended for non-payment", "event", obs.EventBillingStopped, "user_id", userID.String(), "projects", len(res.Projects))
	return res, nil
}
