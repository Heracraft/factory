package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/heracraft/repose/internal/api/ops"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
)

func (s *Server) listSnapshots(w http.ResponseWriter, r *http.Request) error {
	p, err := s.userProjectAny(r)
	if err != nil {
		return err
	}
	snaps, err := store.ListSnapshots(r.Context(), s.d.Pool, p.ID)
	if err != nil {
		return err
	}
	out := make([]map[string]any, 0, len(snaps))
	for _, sn := range snaps {
		out = append(out, map[string]any{"id": sn.ID, "created_at": sn.TakenAt, "bytes": sn.Bytes, "reason": sn.Reason, "expires_at": sn.ExpiresAt})
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) createSnapshot(w http.ResponseWriter, r *http.Request) error {
	p, err := s.userProject(r)
	if err != nil {
		return err
	}
	if p.GuestID == nil || (p.State != "running" && p.State != "stopped") {
		return errf("conflict", "%s is %s; snapshots need a running or stopped guest", p.Slug, p.State)
	}
	pid := p.ID
	id, err := s.enqueue(r.Context(), ops.NewOp{Kind: ops.KindSnapshot, ProjectID: &pid, Params: map[string]any{"reason": "manual"}, Phases: ops.PlanSnapshot()}, false)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"op_id": id})
	return nil
}

func (s *Server) restoreSnapshot(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r.Context())
	ctx := r.Context()
	src, err := s.userProjectAny(r)
	if err != nil {
		return err
	}
	sid, err := pathID(r, "sid")
	if err != nil {
		return err
	}
	snap, err := store.GetSnapshot(ctx, s.d.Pool, sid)
	if err != nil || snap.ProjectID != src.ID || snap.DeletedAt != nil {
		return db.ErrNotFound
	}
	var body struct {
		AsNewProject *string `json:"as_new_project"`
		Start        *bool   `json:"start"`
	}
	if r.ContentLength != 0 {
		if err := decode(r, &body); err != nil {
			return err
		}
	}
	start := body.Start == nil || *body.Start
	if err := billingGate(u); err != nil {
		return err
	}
	target := src
	if body.AsNewProject != nil {
		name := *body.AsNewProject
		if !nameRe.MatchString(name) || Slug(name) == "" {
			return errf("invalid", "as_new_project must match [A-Za-z0-9._-]{1,64}")
		}
		newID := store.NewID()
		rid := store.NewID()
		err := db.InTx(ctx, s.d.Pool, func(tx db.Tx) error {
			var count int
			if err := tx.QueryRow(ctx, "select count(*) from projects where user_id = (select id from users where id = $1 for update) and destroyed_at is null", u.ID).Scan(&count); err != nil {
				return err
			}
			if count >= u.ProjectLimit {
				return withDetail(errf("invalid", "you have %d of %d projects", count, u.ProjectLimit), map[string]any{"limit": u.ProjectLimit})
			}
			_, err := tx.Exec(ctx, `insert into projects (id, user_id, name, slug, class, state, volume_bytes, tz, agent_default, base_version, config_revision_id) values ($1, $2, $3, $4, $5, 'stopped', $6, $7, $8, $9, $10)`,
				newID, u.ID, name, Slug(name), src.Class, src.VolumeBytes, src.TZ, src.AgentDefault, src.BaseVersion, rid)
			if err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23505" {
					return errf("conflict", "a project named %s already exists", Slug(name))
				}
				return err
			}
			if src.ConfigRevisionID != nil {
				cur, err := store.GetRevision(ctx, tx, *src.ConfigRevisionID)
				if err != nil {
					return err
				}
				_, err = tx.Exec(ctx, "insert into config_revisions (id, project_id, fragment, menu, base_version, status, system_closure, closure_bytes, kernel_changed, built_at) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())",
					rid, newID, cur.Fragment, cur.Menu, cur.BaseVersion, revisionStatusForCopy(cur), cur.SystemClosure, cur.ClosureBytes, cur.KernelChanged)
				return err
			}
			_, err = tx.Exec(ctx, "insert into config_revisions (id, project_id, fragment, status) values ($1, $2, $3, 'building')", rid, newID, DefaultFragment)
			return err
		})
		if err != nil {
			return err
		}
		target, err = store.GetProject(ctx, s.d.Pool, newID)
		if err != nil {
			return err
		}
	} else if src.State != "stopped" && src.State != "error" {
		return errf("conflict", "%s must be stopped before a restore replaces its volume", src.Slug)
	}
	hasClosure := false
	if target.ConfigRevisionID != nil {
		if rev, err := store.GetRevision(ctx, s.d.Pool, *target.ConfigRevisionID); err == nil && rev.SystemClosure != nil {
			hasClosure = true
		}
	}
	tid := target.ID
	id, err := s.enqueue(ctx, ops.NewOp{Kind: ops.KindRestore, ProjectID: &tid, SnapshotID: &sid, Params: map[string]any{"start": start}, Phases: ops.PlanRestore(target, hasClosure, start)}, false)
	if err != nil {
		return err
	}
	out := map[string]any{"op_id": id, "project_id": target.ID}
	writeJSON(w, http.StatusAccepted, out)
	return nil
}

func revisionStatusForCopy(cur *store.Revision) string {
	if cur.SystemClosure != nil {
		return "built"
	}
	return "building"
}

var _ = uuid.Nil
