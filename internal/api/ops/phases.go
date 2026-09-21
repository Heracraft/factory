package ops

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/scheduler"
	"github.com/heracraft/repose/internal/api/secrets"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
)

// Phase names. An op's phases are fixed at enqueue by the Plan functions.
const (
	PhaseBuild         = "build"
	PhaseCreateGuest   = "create_guest"
	PhaseStartGuest    = "start_guest"
	PhaseStopGuest     = "stop_guest"
	PhaseSnapshot      = "snapshot"
	PhaseDestroyGuest  = "destroy_guest"
	PhaseResize        = "resize"
	PhaseRestore       = "restore"
	PhaseApplyConfig   = "apply_config"
	PhaseUpdateSecrets = "update_secrets"
	PhaseExec          = "exec"
	PhaseDrain         = "drain"
)

// Op kinds.
const (
	KindCreate        = "create"
	KindStart         = "start"
	KindStop          = "stop"
	KindDestroy       = "destroy"
	KindResize        = "resize"
	KindSnapshot      = "snapshot"
	KindRestore       = "restore"
	KindBuild         = "build"
	KindApply         = "apply"
	KindUpdateSecrets = "update_secrets"
	KindExec          = "exec"
	KindDrain         = "drain"
)

// PlanCreate is build then create.
func PlanCreate() []string { return []string{PhaseBuild, PhaseCreateGuest} }

// PlanStart is start, then apply a built-but-unapplied revision.
func PlanStart(pendingRevision bool) []string {
	if pendingRevision {
		return []string{PhaseStartGuest, PhaseApplyConfig}
	}
	return []string{PhaseStartGuest}
}

// PlanStop is one stop.
func PlanStop() []string { return []string{PhaseStopGuest} }

// PlanDestroy takes a final snapshot (stopping first when running) and
// destroys; a project that never got a guest has nothing to send.
func PlanDestroy(p *store.Project) []string {
	if p.GuestID == nil || p.HostID == nil {
		return []string{}
	}
	switch p.State {
	case "running", "starting", "stopping":
		return []string{PhaseStopGuest, PhaseDestroyGuest}
	default:
		return []string{PhaseSnapshot, PhaseDestroyGuest}
	}
}

// PlanResize is one resize.
func PlanResize() []string { return []string{PhaseResize} }

// PlanSnapshot is one snapshot.
func PlanSnapshot() []string { return []string{PhaseSnapshot} }

// PlanRestore destroys the stopped project's guest (keeping nothing),
// builds when the project has no closure yet, restores, and starts.
func PlanRestore(p *store.Project, hasClosure, start bool) []string {
	var ph []string
	if p.GuestID != nil && p.HostID != nil {
		ph = append(ph, PhaseDestroyGuest)
	}
	if !hasClosure {
		ph = append(ph, PhaseBuild)
	}
	ph = append(ph, PhaseRestore)
	if start {
		ph = append(ph, PhaseStartGuest)
	}
	return ph
}

// PlanBuild builds and, when the guest runs, applies.
func PlanBuild(running bool) []string {
	if running {
		return []string{PhaseBuild, PhaseApplyConfig}
	}
	return []string{PhaseBuild}
}

// PlanApply re-applies a built revision.
func PlanApply() []string { return []string{PhaseApplyConfig} }

// PlanUpdateSecrets pushes the whole set to a running guest.
func PlanUpdateSecrets() []string { return []string{PhaseUpdateSecrets} }

// PlanExec runs one audited command.
func PlanExec() []string { return []string{PhaseExec} }

// PlanDrain drains a host.
func PlanDrain() []string { return []string{PhaseDrain} }

// delivery is what CreateGuest, StartGuest and Restore carry (I-26).
type delivery struct {
	secrets    []*hostdv1.Secret
	hostKey    []byte
	hostCert   []byte
	env        map[string]string
	principals []string
	project    []byte
	// values is every user secret's plaintext, for build-log redaction and
	// the fragment scan; it never leaves this package.
	values []string
}

func (e *Engine) projectAndUser(ctx context.Context, op *store.Op) (*store.Project, *store.User, error) {
	if op.ProjectID == nil {
		return nil, nil, errors.New("op has no project")
	}
	p, err := store.GetProject(ctx, e.pool, *op.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	u, err := store.GetUser(ctx, e.pool, p.UserID)
	if err != nil {
		return nil, nil, err
	}
	return p, u, nil
}

// delivery decrypts the project's secrets for the wire. The reserved
// names hold the guest's sshd material; a missing host key is generated
// and stored under them so every later start delivers the same key.
func (e *Engine) delivery(ctx context.Context, p *store.Project, u *store.User) (*delivery, error) {
	vals, err := e.sec.DecryptForGuest(ctx, p.ID.String())
	if err != nil {
		return nil, err
	}
	d := &delivery{env: map[string]string{"LANG": e.cfg.Lang}, principals: []string{p.ID.String()}}
	for _, v := range vals {
		switch v.Name {
		case "ssh_host_ed25519_key":
			d.hostKey = v.Value
		case "ssh_host_ed25519_key-cert.pub":
			d.hostCert = v.Value
		case "user_ca.pub":
		default:
			d.secrets = append(d.secrets, &hostdv1.Secret{Name: v.Name, Value: v.Value})
			d.values = append(d.values, string(v.Value))
		}
	}
	if d.hostKey == nil || d.hostCert == nil {
		principals := []string{p.Slug + "." + u.Handle}
		if p.GuestIP != nil {
			principals = append([]string{p.GuestIP.String()}, principals...)
		}
		hk, err := e.ca.NewGuestHostKey(ctx, principals)
		if err != nil {
			return nil, err
		}
		if err := e.sec.PutReserved(ctx, p.UserID.String(), p.ID.String(), "ssh_host_ed25519_key", hk.PrivateKeyPEM); err != nil {
			return nil, err
		}
		if err := e.sec.PutReserved(ctx, p.UserID.String(), p.ID.String(), "ssh_host_ed25519_key-cert.pub", []byte(hk.Certificate)); err != nil {
			return nil, err
		}
		d.hostKey, d.hostCert = hk.PrivateKeyPEM, []byte(hk.Certificate)
	}
	tz := "UTC"
	if p.TZ != nil && *p.TZ != "" {
		tz = *p.TZ
	} else if u.TZ != nil && *u.TZ != "" {
		tz = *u.TZ
	}
	d.env["TZ"] = tz
	remote := ""
	if p.RemoteURL != nil {
		remote = *p.RemoteURL
	}
	pj, err := json.Marshal(map[string]any{"project_id": p.ID.String(), "slug": p.Slug, "name": p.Name, "remote_url": remote, "user_handle": u.Handle, "class": p.Class, "tz": tz})
	if err != nil {
		return nil, err
	}
	d.project = pj
	return d, nil
}

// resignHostCert re-signs the guest's host certificate once its address
// is known, so the next start delivers a certificate with both
// principals.
func (e *Engine) resignHostCert(ctx context.Context, p *store.Project, u *store.User, ip string) error {
	vals, err := e.sec.DecryptForGuest(ctx, p.ID.String())
	if err != nil {
		return err
	}
	for _, v := range vals {
		if v.Name == "ssh_host_ed25519_key" {
			signer, err := parseSSHKey(v.Value)
			if err != nil {
				return err
			}
			line, err := e.ca.SignHostCert(ctx, signer, []string{ip, p.Slug + "." + u.Handle}, 10*365*24*time.Hour)
			if err != nil {
				return err
			}
			return e.sec.PutReserved(ctx, p.UserID.String(), p.ID.String(), "ssh_host_ed25519_key-cert.pub", []byte(line))
		}
	}
	return nil
}

// baseRef picks the base a build runs against: the revision's own
// base_version first (a base bump's revision names the new base while the
// project still records the old one; until I-134 the project's base won
// and every bump was built against the base it was leaving), then the
// project's, then the newest published, then the dev checkout.
func (e *Engine) baseRef(ctx context.Context, p *store.Project, rev *store.Revision) (version, ref string, err error) {
	if rev != nil && rev.BaseVersion != nil {
		b, err := store.GetBase(ctx, e.pool, *rev.BaseVersion)
		if err == nil {
			return b.Version, b.NixRev, nil
		}
		if !errors.Is(err, db.ErrNotFound) {
			return "", "", err
		}
	}
	if p.BaseVersion != nil {
		b, err := store.GetBase(ctx, e.pool, *p.BaseVersion)
		if err == nil {
			return b.Version, b.NixRev, nil
		}
		if !errors.Is(err, db.ErrNotFound) {
			return "", "", err
		}
	}
	b, err := store.LatestBase(ctx, e.pool)
	if err == nil {
		return b.Version, b.NixRev, nil
	}
	if errors.Is(err, db.ErrNotFound) {
		if e.cfg.BaseRef == "" {
			return "", "", &opError{code: "internal", msg: "no base version published; run `repose-admin base publish`"}
		}
		return "dev", e.cfg.BaseRef, nil
	}
	return "", "", err
}

func newCommandID() string { return store.NewID().String() }

// buildCommand produces the phase's command (with a fresh command_id the
// caller may overwrite) and the host it goes to. skip means the phase has
// nothing to send.
func (e *Engine) buildCommand(ctx context.Context, op *store.Op, phase string) (cmd *hostdv1.Command, hostID uuid.UUID, skip bool, err error) {
	if phase == PhaseDrain {
		if op.HostID == nil {
			return nil, uuid.Nil, false, errors.New("drain op has no host")
		}
		return &hostdv1.Command{CommandId: newCommandID(), Cmd: &hostdv1.Command_Drain{Drain: &hostdv1.Drain{}}}, *op.HostID, false, nil
	}
	p, u, err := e.projectAndUser(ctx, op)
	if err != nil {
		return nil, uuid.Nil, false, err
	}
	switch phase {
	case PhaseBuild:
		return e.buildBuild(ctx, op, p)
	case PhaseCreateGuest:
		return e.buildCreate(ctx, op, p, u)
	case PhaseStartGuest:
		return e.buildStart(ctx, op, p, u)
	case PhaseStopGuest:
		if p.GuestID == nil || p.HostID == nil {
			return nil, uuid.Nil, true, nil
		}
		snap := true
		if v, ok := op.Params["snapshot"].(bool); ok {
			snap = v
		}
		if err := e.setState(ctx, p, "stopping"); err != nil {
			return nil, uuid.Nil, false, err
		}
		return &hostdv1.Command{CommandId: newCommandID(), Cmd: &hostdv1.Command_StopGuest{StopGuest: &hostdv1.StopGuest{GuestId: p.GuestID.String(), SnapshotFirst: snap, TimeoutS: 60}}}, *p.HostID, false, nil
	case PhaseSnapshot:
		if p.GuestID == nil || p.HostID == nil {
			return nil, uuid.Nil, true, nil
		}
		reason := "manual"
		if r, ok := op.Params["reason"].(string); ok && r != "" {
			reason = r
		}
		return &hostdv1.Command{CommandId: newCommandID(), Cmd: &hostdv1.Command_Snapshot{Snapshot: &hostdv1.Snapshot{GuestId: p.GuestID.String(), Reason: reason}}}, *p.HostID, false, nil
	case PhaseDestroyGuest:
		if p.GuestID == nil || p.HostID == nil {
			return nil, uuid.Nil, true, nil
		}
		if op.Kind == KindDestroy {
			if err := e.setState(ctx, p, "destroying"); err != nil {
				return nil, uuid.Nil, false, err
			}
		}
		return &hostdv1.Command{CommandId: newCommandID(), Cmd: &hostdv1.Command_DestroyGuest{DestroyGuest: &hostdv1.DestroyGuest{GuestId: p.GuestID.String(), KeepVolume: false}}}, *p.HostID, false, nil
	case PhaseResize:
		if p.GuestID == nil || p.HostID == nil {
			return nil, uuid.Nil, false, &opError{code: "invalid", msg: "project has no guest yet"}
		}
		nb, _ := op.Params["volume_bytes"].(float64)
		return &hostdv1.Command{CommandId: newCommandID(), Cmd: &hostdv1.Command_ResizeVolume{ResizeVolume: &hostdv1.ResizeVolume{GuestId: p.GuestID.String(), NewBytes: uint64(nb)}}}, *p.HostID, false, nil
	case PhaseRestore:
		return e.buildRestore(ctx, op, p, u)
	case PhaseApplyConfig:
		return e.buildApply(ctx, op, p)
	case PhaseUpdateSecrets:
		if p.GuestID == nil || p.HostID == nil || p.State != "running" {
			return nil, uuid.Nil, true, nil
		}
		d, err := e.delivery(ctx, p, u)
		if err != nil {
			return nil, uuid.Nil, false, err
		}
		return &hostdv1.Command{CommandId: newCommandID(), Cmd: &hostdv1.Command_UpdateSecrets{UpdateSecrets: &hostdv1.UpdateSecrets{GuestId: p.GuestID.String(), Secrets: d.secrets}}}, *p.HostID, false, nil
	case PhaseExec:
		if p.GuestID == nil || p.HostID == nil {
			return nil, uuid.Nil, false, &opError{code: "invalid", msg: "project has no guest"}
		}
		if op.AuditID == nil {
			return nil, uuid.Nil, false, &opError{code: "invalid", msg: "exec requires an audit id"}
		}
		var argv []string
		if raw, ok := op.Params["argv"].([]any); ok {
			for _, a := range raw {
				if s, ok := a.(string); ok {
					argv = append(argv, s)
				}
			}
		}
		timeout, _ := op.Params["timeout_s"].(float64)
		if timeout == 0 {
			timeout = 60
		}
		return &hostdv1.Command{CommandId: newCommandID(), Cmd: &hostdv1.Command_Exec{Exec: &hostdv1.Exec{GuestId: p.GuestID.String(), Argv: argv, TimeoutS: uint32(timeout), AuditId: op.AuditID.String()}}}, *p.HostID, false, nil
	}
	return nil, uuid.Nil, false, fmt.Errorf("unknown phase %q", phase)
}

func (e *Engine) setState(ctx context.Context, p *store.Project, state string) error {
	if p.State == state {
		return nil
	}
	if err := store.SetProjectState(ctx, e.pool, p.ID, state); err != nil {
		return err
	}
	e.log.Info("project state", "event", "guest_state", "project_id", p.ID.String(), "state", state)
	p.State = state
	return nil
}

func (e *Engine) revisionFor(ctx context.Context, op *store.Op, p *store.Project) (*store.Revision, error) {
	id := op.RevisionID
	if id == nil {
		id = p.ConfigRevisionID
	}
	if id == nil {
		return nil, &opError{code: "invalid", msg: "project has no config revision"}
	}
	return store.GetRevision(ctx, e.pool, *id)
}

func (e *Engine) buildBuild(ctx context.Context, op *store.Op, p *store.Project) (*hostdv1.Command, uuid.UUID, bool, error) {
	rev, err := e.revisionFor(ctx, op, p)
	if err != nil {
		return nil, uuid.Nil, false, err
	}
	// A fragment carrying a current secret value is refused before
	// evaluation (docs/features/secrets.md).
	vals, err := e.sec.DecryptForGuest(ctx, p.ID.String())
	if err != nil {
		return nil, uuid.Nil, false, err
	}
	var redact []string
	for _, v := range vals {
		if secrets.IsReserved(v.Name) {
			continue
		}
		redact = append(redact, string(v.Value))
		if len(v.Value) >= 4 && strings.Contains(rev.Fragment, string(v.Value)) {
			return nil, uuid.Nil, false, &opError{code: "invalid", msg: "fragment contains the value of secret " + v.Name}
		}
	}
	e.logs.SetRedactions(op.ID, redact)
	version, ref, err := e.baseRef(ctx, p, rev)
	if err != nil {
		return nil, uuid.Nil, false, err
	}
	hostID := p.HostID
	if hostID == nil {
		var picked uuid.UUID
		err := db.InTx(ctx, e.pool, func(tx db.Tx) error {
			// An operator may pin the placement (repose-admin hosts smoke).
			if want, ok := op.Params["host_id"].(string); ok && want != "" {
				h, err := store.GetHost(ctx, tx, uuid.MustParse(want))
				if err != nil {
					return err
				}
				if h.State != "ready" && h.State != "draining" {
					return &opError{code: "capacity", msg: "host " + h.Name + " is " + h.State}
				}
				picked = h.ID
			} else {
				pk, err := scheduler.PickHost(ctx, tx, p.Class, p.VolumeBytes, e.now())
				if err != nil {
					return err
				}
				picked = pk.HostID
			}
			_, err := tx.Exec(ctx, "update projects set host_id = $2 where id = $1 and host_id is null", p.ID, picked)
			return err
		})
		if errors.Is(err, scheduler.ErrNoCapacity) {
			// docs/workstreams/10-observability.md §5 requires schedule_fail
			// and counts placements by result; without these two lines a
			// fleet that has run out of memory is invisible until a user
			// complains.
			e.m.ScheduleTotal.WithLabelValues("no_capacity").Inc()
			e.log.Warn("no host has capacity", "event", "schedule_fail",
				"project_id", p.ID.String(), "class", p.Class, "reason", "no_capacity")
			return nil, uuid.Nil, false, errCapacity()
		}
		if err != nil {
			e.m.ScheduleTotal.WithLabelValues("error").Inc()
			e.log.Error("placement failed", "event", "schedule_fail",
				"project_id", p.ID.String(), "class", p.Class, "reason", "error", "err", err.Error())
			return nil, uuid.Nil, false, err
		}
		e.m.ScheduleTotal.WithLabelValues("ok").Inc()
		e.log.Info("placed project", "event", "schedule", "project_id", p.ID.String(), "host_id", picked.String(), "class", p.Class)
		hostID = &picked
		p.HostID = hostID
	}
	if op.Kind == KindCreate {
		if err := e.setState(ctx, p, "building"); err != nil {
			return nil, uuid.Nil, false, err
		}
	}
	if _, err := e.pool.Exec(ctx, "update config_revisions set status = 'building', base_version = coalesce(base_version, $2) where id = $1 and status <> 'building'", rev.ID, version); err != nil {
		return nil, uuid.Nil, false, err
	}
	if rev.BaseVersion == nil {
		if _, err := e.pool.Exec(ctx, "update projects set base_version = coalesce(base_version, $2) where id = $1", p.ID, version); err != nil {
			return nil, uuid.Nil, false, err
		}
	}
	l := e.cfg.Limits
	return &hostdv1.Command{CommandId: newCommandID(), Cmd: &hostdv1.Command_Build{Build: &hostdv1.Build{
		ProjectId: p.ID.String(), RevisionId: rev.ID.String(), Fragment: []byte(rev.Fragment), BaseRef: ref, BaseVersion: version,
		Limits: &hostdv1.Limits{EvalS: l.EvalS, BuildS: l.BuildS, Cores: l.Cores, ClosureBytes: l.ClosureBytes},
	}}}, *hostID, false, nil
}

func (e *Engine) buildCreate(ctx context.Context, op *store.Op, p *store.Project, u *store.User) (*hostdv1.Command, uuid.UUID, bool, error) {
	if p.HostID == nil {
		return nil, uuid.Nil, false, errors.New("create_guest before placement")
	}
	rev, err := e.revisionFor(ctx, op, p)
	if err != nil {
		return nil, uuid.Nil, false, err
	}
	if rev.SystemClosure == nil {
		return nil, uuid.Nil, false, &opError{code: "internal", msg: "revision has no closure"}
	}
	if p.GuestID == nil {
		gid := store.NewID()
		if _, err := e.pool.Exec(ctx, "update projects set guest_id = $2 where id = $1", p.ID, gid); err != nil {
			return nil, uuid.Nil, false, err
		}
		p.GuestID = &gid
	}
	d, err := e.delivery(ctx, p, u)
	if err != nil {
		return nil, uuid.Nil, false, err
	}
	if err := e.setState(ctx, p, "starting"); err != nil {
		return nil, uuid.Nil, false, err
	}
	remote := ""
	if p.RemoteURL != nil {
		remote = *p.RemoteURL
	}
	return &hostdv1.Command{CommandId: newCommandID(), Cmd: &hostdv1.Command_CreateGuest{CreateGuest: &hostdv1.CreateGuest{
		ProjectId: p.ID.String(), GuestId: p.GuestID.String(), Class: p.Class, VolumeBytes: uint64(p.VolumeBytes), SystemClosure: *rev.SystemClosure,
		Secrets: d.secrets, Env: d.env, SshCaPub: e.ca.UserCAPub(), Principals: d.principals, HostKey: d.hostKey, HostCert: d.hostCert,
		UserId: p.UserID.String(), ProjectSlug: p.Slug, RemoteUrl: remote, ProjectJson: d.project,
	}}}, *p.HostID, false, nil
}

func (e *Engine) buildStart(ctx context.Context, op *store.Op, p *store.Project, u *store.User) (*hostdv1.Command, uuid.UUID, bool, error) {
	if p.GuestID == nil || p.HostID == nil {
		return nil, uuid.Nil, false, &opError{code: "invalid", msg: "project has no guest; create it first"}
	}
	d, err := e.delivery(ctx, p, u)
	if err != nil {
		return nil, uuid.Nil, false, err
	}
	if err := e.setState(ctx, p, "starting"); err != nil {
		return nil, uuid.Nil, false, err
	}
	return &hostdv1.Command{CommandId: newCommandID(), Cmd: &hostdv1.Command_StartGuest{StartGuest: &hostdv1.StartGuest{
		GuestId: p.GuestID.String(), Secrets: d.secrets, Env: d.env, SshCaPub: e.ca.UserCAPub(), Principals: d.principals, HostKey: d.hostKey, HostCert: d.hostCert, ProjectJson: d.project,
	}}}, *p.HostID, false, nil
}

func (e *Engine) buildRestore(ctx context.Context, op *store.Op, p *store.Project, u *store.User) (*hostdv1.Command, uuid.UUID, bool, error) {
	if op.SnapshotID == nil {
		return nil, uuid.Nil, false, &opError{code: "invalid", msg: "restore needs a snapshot"}
	}
	snap, err := store.GetSnapshot(ctx, e.pool, *op.SnapshotID)
	if err != nil {
		return nil, uuid.Nil, false, &opError{code: "not_found", msg: "snapshot not found"}
	}
	// Mark the snapshot as in use by this op inside the same lock the
	// expiry job takes, so it is never deleted under a running restore.
	err = db.InTx(ctx, e.pool, func(tx db.Tx) error {
		var deleted *time.Time
		if err := tx.QueryRow(ctx, "select deleted_at from snapshots where id = $1 for update", snap.ID).Scan(&deleted); err != nil {
			return err
		}
		if deleted != nil {
			return &opError{code: "not_found", msg: "snapshot was deleted"}
		}
		_, err := tx.Exec(ctx, "update snapshots set restoring_op_id = $2 where id = $1", snap.ID, op.ID)
		return err
	})
	if err != nil {
		return nil, uuid.Nil, false, err
	}
	// The new guest id is fixed on first build so a re-send is identical.
	gidStr, _ := op.Params["new_guest_id"].(string)
	if gidStr == "" {
		gidStr = store.NewID().String()
		op.Params["new_guest_id"] = gidStr
	}
	// Host: the project's if it can take placements, else pick one.
	hostID := uuid.Nil
	if p.HostID != nil {
		h, err := store.GetHost(ctx, e.pool, *p.HostID)
		if err == nil && h.State == "ready" && !h.Draining {
			hostID = h.ID
		}
	}
	if to, ok := op.Params["to_host_id"].(string); ok && to != "" {
		hostID = uuid.MustParse(to)
	}
	if hostID == uuid.Nil {
		err := db.InTx(ctx, e.pool, func(tx db.Tx) error {
			pk, err := scheduler.PickHost(ctx, tx, p.Class, p.VolumeBytes, e.now())
			if err != nil {
				return err
			}
			hostID = pk.HostID
			return nil
		})
		if errors.Is(err, scheduler.ErrNoCapacity) {
			return nil, uuid.Nil, false, errCapacity()
		}
		if err != nil {
			return nil, uuid.Nil, false, err
		}
	}
	// Whether this restore leaves the project's host, fixed on the first
	// build so a re-send says the same (I-142): the host_moved event is
	// for a project that came up somewhere else, not for a restore over
	// the same host's volume.
	if _, ok := op.Params["host_moved"]; !ok {
		op.Params["host_moved"] = p.HostID == nil || *p.HostID != hostID
	}
	if _, err := e.pool.Exec(ctx, "update projects set host_id = $2, guest_id = $3, guest_ip = null, vsock_cid = null where id = $1", p.ID, hostID, gidStr); err != nil {
		return nil, uuid.Nil, false, err
	}
	p.HostID = &hostID
	gid := uuid.MustParse(gidStr)
	p.GuestID = &gid
	if err := e.setState(ctx, p, "restoring"); err != nil {
		return nil, uuid.Nil, false, err
	}
	d, err := e.delivery(ctx, p, u)
	if err != nil {
		return nil, uuid.Nil, false, err
	}
	closure := ""
	if p.ConfigRevisionID != nil {
		if rev, err := store.GetRevision(ctx, e.pool, *p.ConfigRevisionID); err == nil && rev.SystemClosure != nil {
			closure = *rev.SystemClosure
		}
	}
	if op.RevisionID != nil {
		if rev, err := store.GetRevision(ctx, e.pool, *op.RevisionID); err == nil && rev.SystemClosure != nil {
			closure = *rev.SystemClosure
		}
	}
	remote := ""
	if p.RemoteURL != nil {
		remote = *p.RemoteURL
	}
	return &hostdv1.Command{CommandId: newCommandID(), Cmd: &hostdv1.Command_Restore{Restore: &hostdv1.Restore{
		ProjectId: p.ID.String(), GuestId: gidStr, BlobPath: snap.BlobPath, Class: p.Class, VolumeBytes: uint64(p.VolumeBytes), SystemClosure: closure,
		Secrets: d.secrets, Env: d.env, SshCaPub: e.ca.UserCAPub(), Principals: d.principals, HostKey: d.hostKey, HostCert: d.hostCert,
		UserId: p.UserID.String(), ProjectSlug: p.Slug, RemoteUrl: remote, ProjectJson: d.project,
	}}}, hostID, false, nil
}

func (e *Engine) buildApply(ctx context.Context, op *store.Op, p *store.Project) (*hostdv1.Command, uuid.UUID, bool, error) {
	if p.GuestID == nil || p.HostID == nil || p.State != "running" {
		return nil, uuid.Nil, true, nil // applies at the next start
	}
	// A start applies the newest built-but-unapplied revision, fixed on
	// the op so the result handler sees the same one.
	if op.Kind == KindStart && op.RevisionID == nil {
		var rid uuid.UUID
		err := e.pool.QueryRow(ctx, "select id from config_revisions where project_id = $1 and status = 'built' and system_closure is not null order by created_at desc limit 1", p.ID).Scan(&rid)
		if err != nil {
			if db.IsNoRows(err) {
				return nil, uuid.Nil, true, nil
			}
			return nil, uuid.Nil, false, err
		}
		if _, err := e.pool.Exec(ctx, "update ops set revision_id = $2 where id = $1", op.ID, rid); err != nil {
			return nil, uuid.Nil, false, err
		}
		op.RevisionID = &rid
	}
	rev, err := e.revisionFor(ctx, op, p)
	if err != nil {
		return nil, uuid.Nil, false, err
	}
	if rev.SystemClosure == nil {
		return nil, uuid.Nil, false, &opError{code: "invalid", msg: "revision is not built"}
	}
	if p.ConfigRevisionID != nil && *p.ConfigRevisionID == rev.ID && rev.Status == "applied" && op.Kind != KindApply {
		return nil, uuid.Nil, true, nil
	}
	force, _ := op.Params["reboot"].(bool)
	if op.Kind == KindStart && rev.KernelChanged {
		force = true
	}
	return &hostdv1.Command{CommandId: newCommandID(), Cmd: &hostdv1.Command_ApplyConfig{ApplyConfig: &hostdv1.ApplyConfig{GuestId: p.GuestID.String(), SystemClosure: *rev.SystemClosure, ForceReboot: force}}}, *p.HostID, false, nil
}

// onResult applies a successful phase result.
func (e *Engine) onResult(ctx context.Context, op *store.Op, phase string, res *hostdv1.Result) error {
	if phase == PhaseDrain {
		_, err := e.pool.Exec(ctx, "update hosts set draining = true, state = case when state = 'ready' then 'draining' else state end where id = $1", *op.HostID)
		return err
	}
	p, u, err := e.projectAndUser(ctx, op)
	if err != nil {
		return err
	}
	switch phase {
	case PhaseBuild:
		b := res.GetBuild()
		if b == nil {
			return errors.New("build result without payload")
		}
		rev, err := e.revisionFor(ctx, op, p)
		if err != nil {
			return err
		}
		_, err = e.pool.Exec(ctx, "update config_revisions set status = 'built', system_closure = $2, closure_bytes = $3, kernel_changed = $4, built_at = now(), error = null, fragment_line = null where id = $1",
			rev.ID, b.SystemClosure, int64(b.ClosureBytes), b.KernelChanged)
		return err
	case PhaseCreateGuest, PhaseRestore:
		c := res.GetCreate()
		if c == nil {
			return errors.New("create result without payload")
		}
		if _, err := e.pool.Exec(ctx, "update projects set guest_ip = $2, vsock_cid = $3, host_unreachable = false, last_error = null where id = $1", p.ID, c.GuestIp, int32(c.VsockCid)); err != nil {
			return err
		}
		if err := e.resignHostCert(ctx, p, u, c.GuestIp); err != nil {
			return err
		}
		if phase == PhaseCreateGuest {
			rev, err := e.revisionFor(ctx, op, p)
			if err != nil {
				return err
			}
			if _, err := e.pool.Exec(ctx, "update config_revisions set status = 'applied', applied_at = now() where id = $1", rev.ID); err != nil {
				return err
			}
			if _, err := e.pool.Exec(ctx, "update projects set config_revision_id = $2 where id = $1", p.ID, rev.ID); err != nil {
				return err
			}
			if err := e.setState(ctx, p, "running"); err != nil {
				return err
			}
		} else {
			if _, err := e.pool.Exec(ctx, "update snapshots set restoring_op_id = null where restoring_op_id = $1", op.ID); err != nil {
				return err
			}
			if err := e.setState(ctx, p, "stopped"); err != nil {
				return err
			}
			if moved, _ := op.Params["host_moved"].(bool); moved {
				e.log.Info("project restored onto another host", "event", "host_moved", "project_id", p.ID.String(), "host_id", p.HostID.String())
				e.notifyPlatform(ctx, p.ID, "host_moved", p.Slug+" was restored onto a new host from its latest snapshot")
			} else {
				e.log.Info("project restored", "event", "restored", "project_id", p.ID.String(), "host_id", p.HostID.String())
			}
		}
		return e.setResult(ctx, op, map[string]any{"guest_ip": c.GuestIp})
	case PhaseStartGuest:
		if _, err := e.pool.Exec(ctx, "update projects set host_unreachable = false, last_error = null where id = $1", p.ID); err != nil {
			return err
		}
		return e.setState(ctx, p, "running")
	case PhaseStopGuest:
		if s := res.GetStop(); s != nil && s.BlobPath != "" {
			if err := e.recordSnapshot(ctx, op, p, s.BlobPath, int64(s.Bytes), "stop"); err != nil {
				return err
			}
		}
		if op.Kind == KindDestroy {
			return nil
		}
		return e.setState(ctx, p, "stopped")
	case PhaseSnapshot:
		s := res.GetSnapshot()
		if s == nil || s.BlobPath == "" {
			return errors.New("snapshot result without a blob path")
		}
		reason := "manual"
		if r, ok := op.Params["reason"].(string); ok && r != "" {
			reason = r
		}
		return e.recordSnapshot(ctx, op, p, s.BlobPath, int64(s.Bytes), reason)
	case PhaseDestroyGuest:
		if op.Kind != KindDestroy {
			return nil // restore: the old guest is gone, the new one follows
		}
		return e.markDestroyed(ctx, p.ID)
	case PhaseResize:
		nb, _ := op.Params["volume_bytes"].(float64)
		_, err := e.pool.Exec(ctx, "update projects set volume_bytes = $2 where id = $1", p.ID, int64(nb))
		return err
	case PhaseApplyConfig:
		a := res.GetApply()
		rev, err := e.revisionFor(ctx, op, p)
		if err != nil {
			return err
		}
		if a != nil && a.RebootRequired {
			op.RebootRequired = true
			if _, err := e.pool.Exec(ctx, "update ops set reboot_required = true where id = $1", op.ID); err != nil {
				return err
			}
			_, err := e.pool.Exec(ctx, "update config_revisions set reboot_required = true where id = $1", rev.ID)
			return err
		}
		if _, err := e.pool.Exec(ctx, "update config_revisions set status = 'applied', applied_at = now(), reboot_required = false where id = $1", rev.ID); err != nil {
			return err
		}
		if _, err := e.pool.Exec(ctx, "update config_revisions set status = 'built' where project_id = $1 and id <> $2 and status = 'applied'", p.ID, rev.ID); err != nil {
			return err
		}
		_, err = e.pool.Exec(ctx, "update projects set config_revision_id = $2, base_version = coalesce($3, base_version) where id = $1", p.ID, rev.ID, rev.BaseVersion)
		if err != nil {
			return err
		}
		e.log.Info("configuration applied", "event", "switch_done", "project_id", p.ID.String(), "rebooted", a != nil && a.Rebooted)
		return nil
	case PhaseUpdateSecrets:
		return nil
	case PhaseExec:
		x := res.GetExec()
		if x == nil {
			return errors.New("exec result without payload")
		}
		return e.setResult(ctx, op, map[string]any{"exit_code": x.ExitCode, "stdout": base64.StdEncoding.EncodeToString(x.Stdout), "stderr": base64.StdEncoding.EncodeToString(x.Stderr)})
	}
	return nil
}

func (e *Engine) recordSnapshot(ctx context.Context, op *store.Op, p *store.Project, blobPath string, bytes int64, reason string) error {
	id := store.NewID()
	var expires *time.Time
	if op.Kind == KindDestroy {
		t := e.now().Add(30 * 24 * time.Hour)
		expires = &t
	}
	var got uuid.UUID
	err := e.pool.QueryRow(ctx, `insert into snapshots (id, project_id, host_id, blob_path, bytes, reason, taken_at, expires_at) values ($1, $2, $3, $4, $5, $6, now(), $7)
		on conflict (blob_path) do update set reason = excluded.reason, bytes = greatest(snapshots.bytes, excluded.bytes), expires_at = coalesce(excluded.expires_at, snapshots.expires_at)
		returning id`, id, p.ID, p.HostID, blobPath, bytes, reason, expires).Scan(&got)
	if err != nil {
		return err
	}
	if _, err := e.pool.Exec(ctx, "update ops set snapshot_id = $2 where id = $1", op.ID, got); err != nil {
		return err
	}
	e.log.Info("snapshot recorded", "event", "snapshot_done", "project_id", p.ID.String(), "reason", reason, "bytes", bytes)
	return e.setResult(ctx, op, map[string]any{"snapshot_id": got.String()})
}

// onFail applies the kind's failure policy to the project.
func (e *Engine) onFail(ctx context.Context, op *store.Op, code, msg string, line int) {
	if op.ProjectID == nil {
		return
	}
	p, err := store.GetProject(ctx, e.pool, *op.ProjectID)
	if err != nil {
		return
	}
	phase := currentPhase(op)
	if phase == PhaseBuild {
		if op.RevisionID != nil || p.ConfigRevisionID != nil {
			rev, err := e.revisionFor(ctx, op, p)
			if err == nil {
				var l *int
				if line > 0 {
					l = &line
				}
				_, _ = e.pool.Exec(ctx, "update config_revisions set status = 'failed', error = $2, fragment_line = $3 where id = $1", rev.ID, code+": "+msg, l) // best effort; the op carries the error
			}
		}
		if op.StartedAt != nil {
			e.m.BuildDuration.WithLabelValues(code).Observe(e.now().Sub(*op.StartedAt).Seconds())
		}
	}
	if phase == PhaseRestore {
		_, _ = e.pool.Exec(ctx, "update snapshots set restoring_op_id = null where restoring_op_id = $1", op.ID) // release the expiry guard
	}
	short := msg
	if i := strings.IndexByte(short, '\n'); i > 0 {
		short = short[:i]
	}
	if len(short) > 200 {
		short = short[:200]
	}
	switch op.Kind {
	case KindCreate, KindStart, KindStop, KindDestroy, KindRestore:
		if code == "capacity" {
			_, _ = e.pool.Exec(ctx, "update projects set host_id = null where id = $1", p.ID) // free the placement
		}
		_, _ = e.pool.Exec(ctx, "update projects set state = 'error', last_error = $2 where id = $1 and state <> 'destroyed'", p.ID, code+": "+short)
	case KindSnapshot:
		_, _ = e.pool.Exec(ctx, "update projects set last_error = $2 where id = $1", p.ID, code+": "+short)
		e.notifyPlatform(ctx, p.ID, "snapshot_failed", "snapshot failed: "+short)
	default:
		_, _ = e.pool.Exec(ctx, "update projects set last_error = $2 where id = $1", p.ID, code+": "+short)
	}
}

// notifyPlatform records a platform-originated event (13-notifications.md
// §2), best effort: a failure to notify must never fail the op it reports.
func (e *Engine) notifyPlatform(ctx context.Context, projectID uuid.UUID, kind, summary string) {
	if e.events == nil {
		return
	}
	if err := e.events.Platform(ctx, projectID, kind, summary); err != nil {
		e.log.Warn("platform event", "event", "notify_fail", "kind", kind, "project_id", projectID.String(), "err", err.Error())
	}
}

// markDestroyed is the end of a destroy: the row keeps its history with
// destroyed_at set and the last snapshot expires in 30 days (R4-11). It
// runs from the destroy_guest phase, and from finish for a destroy whose
// plan was empty because the project never had a guest (a create that
// failed before CreateGuest), which used to leave the project in `error`
// for ever (I-124).
func (e *Engine) markDestroyed(ctx context.Context, projectID uuid.UUID) error {
	_, err := e.pool.Exec(ctx, `update projects set state = 'destroyed', destroyed_at = now(), host_id = null, guest_id = null, guest_ip = null, vsock_cid = null where id = $1 and destroyed_at is null`, projectID)
	if err != nil {
		return err
	}
	e.log.Info("project destroyed", "event", "guest_destroy", "project_id", projectID.String())
	_, err = e.pool.Exec(ctx, "update snapshots set expires_at = coalesce(expires_at, now() + interval '30 days') where project_id = $1 and deleted_at is null", projectID)
	return err
}
