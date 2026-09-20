package httpapi

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/heracraft/repose/internal/api/meter"
	"github.com/heracraft/repose/internal/api/ops"
	"github.com/heracraft/repose/internal/api/scheduler"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/obs"
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
var slugClean = regexp.MustCompile(`[^a-z0-9-]+`)
var slugDashes = regexp.MustCompile(`-{2,}`)

// Slug derives the SSH login half from a project name (docs/features/
// projects.md): lowercase, [a-z0-9-], runs collapsed, 1 to 40 characters.
func Slug(name string) string {
	s := strings.ToLower(name)
	s = slugClean.ReplaceAllString(s, "-")
	s = slugDashes.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	return s
}

// DefaultFragment is the empty user fragment a new project starts with.
const DefaultFragment = "{ pkgs, ... }:\n{\n  home.packages = [ ];\n}\n"

type projectExtras struct {
	costToday, costMonth int64
	lastSnapshot         *time.Time
	latest               *meter.Latest
}

func (s *Server) extras(ctx context.Context, p *store.Project, tz string) (projectExtras, error) {
	var x projectExtras
	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	now := time.Now().In(loc)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	if err := s.d.Pool.QueryRow(ctx, `select coalesce(sum(cost_cents) filter (where hour >= $2), 0), coalesce(sum(cost_cents) filter (where hour >= $3), 0) from usage_hours where project_id = $1`, p.ID, dayStart, monthStart).Scan(&x.costToday, &x.costMonth); err != nil {
		return x, err
	}
	if err := s.d.Pool.QueryRow(ctx, "select max(taken_at) from snapshots where project_id = $1 and deleted_at is null", p.ID).Scan(&x.lastSnapshot); err != nil {
		return x, err
	}
	l, ok, err := meter.LatestSample(ctx, s.d.Pool, p.ID)
	if err != nil {
		return x, err
	}
	if ok {
		x.latest = l
	}
	return x, nil
}

func (s *Server) projectJSON(ctx context.Context, p *store.Project, u *store.User) (map[string]any, error) {
	tz := ""
	if u.TZ != nil {
		tz = *u.TZ
	}
	x, err := s.extras(ctx, p, tz)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"id": p.ID, "name": p.Name, "slug": p.Slug, "remote_url": p.RemoteURL, "class": p.Class, "state": p.State,
		"host_id": p.HostID, "guest_ip": nil, "agent_default": p.AgentDefault, "hold_base_updates": p.HoldBaseUpdates,
		"base_version": p.BaseVersion, "config_revision_id": p.ConfigRevisionID, "volume_bytes": p.VolumeBytes,
		"created_at": p.CreatedAt, "started_at": p.StartedAt, "cost_today_cents": x.costToday, "cost_month_cents": x.costMonth,
		"last_snapshot_at": x.lastSnapshot, "host_unreachable": p.HostUnreachable, "last_error": p.LastError, "tz": p.TZ,
	}
	if p.GuestIP != nil {
		out["guest_ip"] = p.GuestIP.String()
	}
	if x.latest != nil {
		out["disk_used_bytes"] = x.latest.DiskUsed
		agents := []map[string]string{}
		for _, a := range x.latest.Agents {
			agents = append(agents, map[string]string{"agent": a["agent"], "window": a["window"], "state": a["state"]})
		}
		out["signals"] = map[string]any{"ssh_sessions": x.latest.SSHSessions, "tmux_clients": x.latest.TmuxClients, "agents": agents,
			"docker_containers": x.latest.DockerContainers, "guestd_ok": x.latest.GuestdOK, "sampled_at": x.latest.TS, "gateway_sessions": s.sessions.Count(p.ID)}
	}
	return out, nil
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r.Context())
	projects, err := store.ListUserProjects(r.Context(), s.d.Pool, u.ID)
	if err != nil {
		return err
	}
	out := make([]map[string]any, 0, len(projects))
	for i := range projects {
		j, err := s.projectJSON(r.Context(), &projects[i], u)
		if err != nil {
			return err
		}
		out = append(out, j)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) userProject(r *http.Request) (*store.Project, error) {
	id, err := pathID(r, "id")
	if err != nil {
		return nil, err
	}
	return store.GetUserProject(r.Context(), s.d.Pool, userFrom(r.Context()).ID, id)
}

// billingGate is the card and status check before compute (R2-10, I-16).
func billingGate(u *store.User) error {
	if u.BillingStatus == "exempt" {
		return nil
	}
	switch u.BillingStatus {
	case "past_due":
		return withDetail(errf("payment_required", "your account is past due; update your card"), map[string]any{"reason": "past_due"})
	case "suspended":
		return withDetail(errf("payment_required", "your account is suspended"), map[string]any{"reason": "suspended"})
	}
	if !u.HasCard {
		return withDetail(errf("payment_required", "add a card before starting a guest"), map[string]any{"reason": "card_required"})
	}
	if u.BillingStatus == "trial" && u.TrialCreditCents <= 0 {
		return withDetail(errf("payment_required", "your trial credit is used up"), map[string]any{"reason": "trial_depleted"})
	}
	return nil
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r.Context())
	ctx := r.Context()
	var body struct {
		Name      string  `json:"name"`
		RemoteURL *string `json:"remote_url"`
		Class     string  `json:"class"`
		TZ        *string `json:"tz"`
		Agent     *string `json:"agent_default"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if !nameRe.MatchString(body.Name) {
		return errf("invalid", "name must match [A-Za-z0-9._-]{1,64}")
	}
	if body.Class == "" {
		body.Class = "large"
	}
	if !scheduler.ValidClass(body.Class) {
		return errf("invalid", "class must be small, large or xl")
	}
	slug := Slug(body.Name)
	if slug == "" {
		return errf("invalid", "name has no usable characters for a slug")
	}
	if body.TZ != nil {
		if _, err := time.LoadLocation(*body.TZ); err != nil {
			return errf("invalid", "tz is not an IANA zone name")
		}
	}
	agent := "claude"
	if body.Agent != nil {
		agent = *body.Agent
	}
	if err := billingGate(u); err != nil {
		return err
	}
	if u.CancelledAt != nil {
		return errf("forbidden", "account is cancelled")
	}
	pid := store.NewID()
	rid := store.NewID()
	var opID uuid.UUID
	err := db.InTx(ctx, s.d.Pool, func(tx db.Tx) error {
		// Limits are checked under a row lock on the user so two creates
		// cannot both pass.
		var count, xl int
		if err := tx.QueryRow(ctx, "select count(*), count(*) filter (where class = 'xl') from projects where user_id = (select id from users where id = $1 for update) and destroyed_at is null", u.ID).Scan(&count, &xl); err != nil {
			return err
		}
		if count >= u.ProjectLimit {
			return withDetail(errf("invalid", "you have %d of %d projects; destroy one or add a card and pay your first invoice to raise the limit", count, u.ProjectLimit), map[string]any{"limit": u.ProjectLimit, "projects": count})
		}
		if body.Class == "xl" && xl >= u.XLLimit {
			return withDetail(errf("invalid", "you have %d of %d xl projects", xl, u.XLLimit), map[string]any{"xl_limit": u.XLLimit, "xl": xl})
		}
		_, err := tx.Exec(ctx, `insert into projects (id, user_id, name, slug, remote_url, class, state, volume_bytes, tz, agent_default, config_revision_id) values ($1, $2, $3, $4, $5, $6, 'creating', $7, $8, $9, $10)`,
			pid, u.ID, body.Name, slug, body.RemoteURL, body.Class, scheduler.DefaultVolume(body.Class), body.TZ, agent, rid)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				if strings.Contains(pgErr.ConstraintName, "remote") {
					return errf("conflict", "a project for this remote already exists")
				}
				return errf("conflict", "a project named %s already exists", slug)
			}
			return err
		}
		if _, err := tx.Exec(ctx, "insert into config_revisions (id, project_id, fragment, status) values ($1, $2, $3, 'building')", rid, pid, DefaultFragment); err != nil {
			return err
		}
		opID, err = s.d.Engine.Enqueue(ctx, tx, ops.NewOp{Kind: ops.KindCreate, ProjectID: &pid, Phases: ops.PlanCreate()}, false)
		return err
	})
	if err != nil {
		return err
	}
	s.d.Engine.Kick()
	obs.Logger(ctx, s.d.Log).Info("project created", "event", "project_create", "project_id", pid.String(), "class", body.Class)
	p, err := store.GetProject(ctx, s.d.Pool, pid)
	if err != nil {
		return err
	}
	j, err := s.projectJSON(ctx, p, u)
	if err != nil {
		return err
	}
	j["op_id"] = opID
	writeJSON(w, http.StatusCreated, j)
	return nil
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) error {
	p, err := s.userProject(r)
	if err != nil {
		return err
	}
	j, err := s.projectJSON(r.Context(), p, userFrom(r.Context()))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, j)
	return nil
}

func (s *Server) patchProject(w http.ResponseWriter, r *http.Request) error {
	p, err := s.userProject(r)
	if err != nil {
		return err
	}
	var body struct {
		Class *string `json:"class"`
		Hold  *bool   `json:"hold_base_updates"`
		Agent *string `json:"agent_default"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	ctx := r.Context()
	if body.Class != nil {
		if !scheduler.ValidClass(*body.Class) {
			return errf("invalid", "class must be small, large or xl")
		}
		if p.State != "stopped" {
			return errf("conflict", "changing the class requires the project to be stopped")
		}
		if *body.Class == "xl" && p.Class != "xl" {
			u := userFrom(ctx)
			var xl int
			if err := s.d.Pool.QueryRow(ctx, "select count(*) from projects where user_id = $1 and destroyed_at is null and class = 'xl'", u.ID).Scan(&xl); err != nil {
				return err
			}
			if xl >= u.XLLimit {
				return withDetail(errf("invalid", "you have %d of %d xl projects", xl, u.XLLimit), map[string]any{"xl_limit": u.XLLimit})
			}
		}
		if _, err := s.d.Pool.Exec(ctx, "update projects set class = $2 where id = $1 and state = 'stopped'", p.ID, *body.Class); err != nil {
			return err
		}
	}
	if body.Hold != nil {
		if _, err := s.d.Pool.Exec(ctx, "update projects set hold_base_updates = $2 where id = $1", p.ID, *body.Hold); err != nil {
			return err
		}
	}
	if body.Agent != nil {
		if _, err := s.d.Pool.Exec(ctx, "update projects set agent_default = $2 where id = $1", p.ID, *body.Agent); err != nil {
			return err
		}
	}
	fresh, err := store.GetProject(ctx, s.d.Pool, p.ID)
	if err != nil {
		return err
	}
	j, err := s.projectJSON(ctx, fresh, userFrom(ctx))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, j)
	return nil
}

func (s *Server) enqueue(ctx context.Context, n ops.NewOp, allowQueue bool) (uuid.UUID, error) {
	var id uuid.UUID
	err := db.InTx(ctx, s.d.Pool, func(tx db.Tx) error {
		var err error
		id, err = s.d.Engine.Enqueue(ctx, tx, n, allowQueue)
		return err
	})
	if err != nil {
		return uuid.Nil, err
	}
	s.d.Engine.Kick()
	return id, nil
}

func (s *Server) destroyProject(w http.ResponseWriter, r *http.Request) error {
	p, err := s.userProject(r)
	if err != nil {
		return err
	}
	pid := p.ID
	id, err := s.enqueue(r.Context(), ops.NewOp{Kind: ops.KindDestroy, ProjectID: &pid, Phases: ops.PlanDestroy(p)}, false)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"op_id": id})
	return nil
}

func (s *Server) startProject(w http.ResponseWriter, r *http.Request) error {
	p, err := s.userProject(r)
	if err != nil {
		return err
	}
	u := userFrom(r.Context())
	if err := billingGate(u); err != nil {
		return err
	}
	switch p.State {
	case "running", "starting":
		return errf("conflict", "%s is already %s", p.Slug, p.State)
	case "stopped", "error":
	default:
		return errf("conflict", "%s is %s; wait for it to settle", p.Slug, p.State)
	}
	if p.HostID != nil {
		h, err := store.GetHost(r.Context(), s.d.Pool, *p.HostID)
		if err == nil && (h.State == "unreachable" || h.State == "retired" || h.State == "lost") {
			return withDetail(errf("conflict", "%s's host is %s; restore its latest snapshot onto another host", p.Slug, h.State), map[string]any{"host_state": h.State})
		}
	}
	var pending bool
	if err := s.d.Pool.QueryRow(r.Context(), "select exists(select 1 from config_revisions where project_id = $1 and status = 'built' and system_closure is not null)", p.ID).Scan(&pending); err != nil {
		return err
	}
	pid := p.ID
	id, err := s.enqueue(r.Context(), ops.NewOp{Kind: ops.KindStart, ProjectID: &pid, Phases: ops.PlanStart(pending)}, false)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"op_id": id})
	return nil
}

func (s *Server) stopProject(w http.ResponseWriter, r *http.Request) error {
	p, err := s.userProject(r)
	if err != nil {
		return err
	}
	body := struct {
		Snapshot *bool `json:"snapshot"`
	}{}
	if r.ContentLength != 0 {
		if err := decode(r, &body); err != nil {
			return err
		}
	}
	snapshot := body.Snapshot == nil || *body.Snapshot
	switch p.State {
	case "running", "starting", "error":
	case "stopped":
		return errf("conflict", "%s is already stopped", p.Slug)
	default:
		return errf("conflict", "%s is %s; wait for it to settle", p.Slug, p.State)
	}
	pid := p.ID
	id, err := s.enqueue(r.Context(), ops.NewOp{Kind: ops.KindStop, ProjectID: &pid, Params: map[string]any{"snapshot": snapshot}, Phases: ops.PlanStop()}, false)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"op_id": id})
	return nil
}

func opJSON(r *http.Request, op *store.Op) map[string]any {
	out := map[string]any{"op_id": op.ID, "kind": op.Kind, "state": op.State, "created_at": op.CreatedAt, "finished_at": op.FinishedAt, "reboot_required": op.RebootRequired}
	if op.Error != nil {
		out["error"] = op.Error
	}
	if op.Result != nil {
		out["result"] = op.Result
	}
	if op.Kind == ops.KindBuild || op.Kind == ops.KindCreate {
		out["log_url"] = "/v1/projects/" + r.PathValue("id") + "/ops/" + op.ID.String() + "/log"
	}
	return out
}

func (s *Server) userOp(r *http.Request) (*store.Op, error) {
	p, err := s.userProjectAny(r)
	if err != nil {
		return nil, err
	}
	opID, err := pathID(r, "op_id")
	if err != nil {
		return nil, err
	}
	op, err := store.GetOp(r.Context(), s.d.Pool, opID)
	if err != nil {
		return nil, err
	}
	if op.ProjectID == nil || *op.ProjectID != p.ID {
		return nil, db.ErrNotFound
	}
	return op, nil
}

func (s *Server) userProjectAny(r *http.Request) (*store.Project, error) {
	id, err := pathID(r, "id")
	if err != nil {
		return nil, err
	}
	return store.GetUserProjectAny(r.Context(), s.d.Pool, userFrom(r.Context()).ID, id)
}

func (s *Server) getOp(w http.ResponseWriter, r *http.Request) error {
	op, err := s.userOp(r)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, opJSON(r, op))
	return nil
}

func (s *Server) resizeProject(w http.ResponseWriter, r *http.Request) error {
	p, err := s.userProject(r)
	if err != nil {
		return err
	}
	var body struct {
		VolumeBytes int64 `json:"volume_bytes"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.VolumeBytes <= p.VolumeBytes {
		return withDetail(errf("invalid", "volumes only grow; current size is %d bytes", p.VolumeBytes), map[string]any{"volume_bytes": p.VolumeBytes})
	}
	if body.VolumeBytes > 2<<40 {
		return errf("invalid", "volume may not exceed 2 TB")
	}
	if p.GuestID == nil {
		return errf("conflict", "%s has no guest yet", p.Slug)
	}
	pid := p.ID
	id, err := s.enqueue(r.Context(), ops.NewOp{Kind: ops.KindResize, ProjectID: &pid, Params: map[string]any{"volume_bytes": float64(body.VolumeBytes)}, Phases: ops.PlanResize()}, false)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"op_id": id})
	return nil
}

func (s *Server) projectRoute(w http.ResponseWriter, r *http.Request) error {
	p, err := s.userProject(r)
	if err != nil {
		return err
	}
	out := map[string]any{"host_id": p.HostID, "guest_ip": nil, "state": p.State, "host_unreachable": p.HostUnreachable}
	if p.GuestIP != nil {
		out["guest_ip"] = p.GuestIP.String()
	}
	if p.HostID != nil {
		if h, err := store.GetHost(r.Context(), s.d.Pool, *p.HostID); err == nil {
			out["host_name"] = h.Name
			out["host_state"] = h.State
		}
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}
