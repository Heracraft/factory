package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/api/ops"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/obs"
)

// Why the api stopped a user's machines by itself; the metric's label.
const (
	StopReasonPastDue = "past_due" // three days past due (dunning)
	StopReasonEnded   = "ended"    // the subscription was cancelled
	StopReasonEgress  = "egress"   // four times the egress allowance
)

// stopUserMachines snapshots and stops every running or starting project
// of a user (reason billing), the way dunning always did. Projects with an
// op in progress are left for the next run. It returns the projects it
// enqueued stops for.
func stopUserMachines(ctx context.Context, pool *db.Pool, stop Stopper, m *metrics.M, log *slog.Logger, userID uuid.UUID, why string) ([]uuid.UUID, error) {
	projects, err := store.ListUserProjects(ctx, pool, userID)
	if err != nil {
		return nil, err
	}
	var stopped []uuid.UUID
	for i := range projects {
		p := &projects[i]
		if p.State != "running" && p.State != "starting" {
			continue
		}
		pid := p.ID
		err := db.InTx(ctx, pool, func(tx db.Tx) error {
			_, err := stop.Enqueue(ctx, tx, ops.NewOp{
				Kind: ops.KindStop, ProjectID: &pid, Phases: ops.PlanStop(),
				Params: map[string]any{"snapshot": true, "reason": "billing"},
			}, false)
			return err
		})
		if errors.Is(err, ops.ErrOpInProgress) {
			continue
		}
		if err != nil {
			return stopped, fmt.Errorf("stop project %s (%s): %w", pid, why, err)
		}
		stopped = append(stopped, pid)
		if m != nil {
			m.BillingStopsTotal.WithLabelValues(why).Inc()
		}
		log.Warn("machine stopped by billing", "event", obs.EventBillingStopped, "user_id", userID.String(), "project_id", pid.String(), "reason", why)
	}
	if len(stopped) > 0 {
		stop.Kick()
	}
	return stopped, nil
}
