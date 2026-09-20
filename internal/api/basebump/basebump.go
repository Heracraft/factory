// Package basebump applies a published base version to every project not
// holding (05-control-plane-api.md §5.7, DECISIONS R4-5): a build op per
// project reusing its current fragment, failures recorded as
// base_update_failed events without stopping the sweep.
package basebump

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/events"
	"github.com/heracraft/repose/internal/api/ops"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
)

// Job sweeps projects behind the latest base.
type Job struct {
	pool   *db.Pool
	engine *ops.Engine
	events *events.Ingest
	log    *slog.Logger
}

// New builds the job.
func New(pool *db.Pool, engine *ops.Engine, ev *events.Ingest, log *slog.Logger) *Job {
	return &Job{pool: pool, engine: engine, events: ev, log: log.With("component", "api")}
}

// Run sweeps daily at 04:00 UTC and immediately when a security release
// is newer than the last sweep, until ctx ends.
func (j *Job) Run(ctx context.Context) {
	for {
		next := nextRun(time.Now().UTC())
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(next)):
		case <-time.After(10 * time.Minute):
			b, err := store.LatestBase(ctx, j.pool)
			if err != nil || !b.Security || time.Since(b.ReleasedAt) > 10*time.Minute {
				continue
			}
		}
		release, ok, err := db.TryLock(ctx, j.pool, db.LockBaseBump)
		if err != nil || !ok {
			continue
		}
		if _, err := j.Sweep(ctx); err != nil && ctx.Err() == nil {
			j.log.Error("base bump sweep", "event", "base_bump_fail", "err", err.Error())
		}
		release()
	}
}

func nextRun(now time.Time) time.Time {
	n := time.Date(now.Year(), now.Month(), now.Day(), 4, 0, 0, 0, time.UTC)
	if !n.After(now) {
		n = n.AddDate(0, 0, 1)
	}
	return n
}

// Sweep enqueues a build for every unheld project behind the latest base
// and returns the project ids it touched. A project with an open op or
// whose last build failed is skipped until its next successful apply.
func (j *Job) Sweep(ctx context.Context) ([]uuid.UUID, error) {
	latest, err := store.LatestBase(ctx, j.pool)
	if err != nil {
		if db.IsNoRows(err) || err == db.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	rows, err := j.pool.Query(ctx, `select id from projects where destroyed_at is null and state in ('running','stopped') and not hold_base_updates
		and (base_version is null or base_version <> $1) and config_revision_id is not null and user_id <> '00000000-0000-7000-8000-000000000000' order by created_at`, latest.Version)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	var touched []uuid.UUID
	for _, pid := range ids {
		p, err := store.GetProject(ctx, j.pool, pid)
		if err != nil {
			continue
		}
		cur, err := store.GetRevision(ctx, j.pool, *p.ConfigRevisionID)
		if err != nil {
			continue
		}
		// Skip a project whose newest revision failed (the user fixes it
		// first) or is already on the latest base and waiting to apply at
		// the next start.
		revs, _ := store.ListRevisions(ctx, j.pool, pid)
		if len(revs) > 0 {
			if revs[0].Status == "failed" {
				continue
			}
			if revs[0].BaseVersion != nil && *revs[0].BaseVersion == latest.Version && revs[0].Status != "applied" {
				continue
			}
		}
		rid := store.NewID()
		err = db.InTx(ctx, j.pool, func(tx db.Tx) error {
			if _, err := tx.Exec(ctx, "insert into config_revisions (id, project_id, fragment, menu, base_version, status) values ($1, $2, $3, $4, $5, 'building')", rid, pid, cur.Fragment, cur.Menu, latest.Version); err != nil {
				return err
			}
			_, err := j.engine.Enqueue(ctx, tx, ops.NewOp{Kind: ops.KindBuild, ProjectID: &pid, RevisionID: &rid, Params: map[string]any{"base_bump": latest.Version}, Phases: ops.PlanBuild(p.State == "running")}, false)
			return err
		})
		if err != nil {
			j.log.Warn("base bump skipped", "event", "base_bump_skip", "project_id", pid.String(), "err", err.Error())
			continue
		}
		touched = append(touched, pid)
	}
	j.engine.Kick()
	j.log.Info("base bump sweep", "event", "base_bump", "version", latest.Version, "projects", len(touched))
	return touched, nil
}

// OnOpFinished records the outcome of a bump build as an event; wired
// into the engine's OnFinished.
func (j *Job) OnOpFinished(ctx context.Context, op *store.Op) {
	v, ok := op.Params["base_bump"].(string)
	if !ok || op.ProjectID == nil || j.events == nil {
		return
	}
	if op.State == "done" {
		_ = j.events.Platform(ctx, *op.ProjectID, "base_updated", "base "+v+" applied") // best effort; the revision row carries the truth
		return
	}
	msg := "base " + v + " failed to build"
	if op.Error != nil {
		if m, ok := op.Error["message"].(string); ok {
			msg += ": " + m
		}
	}
	_ = j.events.Platform(ctx, *op.ProjectID, "base_update_failed", msg) // see above
}
