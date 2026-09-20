package guest

import (
	"context"
	"errors"
	"time"

	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
	"github.com/heracraft/repose/internal/hostd/nixbuild"
	"github.com/heracraft/repose/internal/hostd/state"
	"github.com/heracraft/repose/internal/hostd/vsockclient"
)

func (m *Manager) build(ctx context.Context, commandID string, c *hostdv1.Build) (*hostdv1.BuildResult, *Error) {
	if c.ProjectId == "" || c.RevisionId == "" {
		return nil, errf(CodeInvalidArgument, "project_id and revision_id required")
	}
	if len(c.Fragment) == 0 {
		return nil, errf(CodeInvalidArgument, "fragment required")
	}
	lim := c.Limits
	if lim == nil {
		lim = &hostdv1.Limits{}
	}
	if lim.EvalS == 0 {
		lim.EvalS = 60
	}
	if lim.BuildS == 0 {
		lim.BuildS = 1800
	}
	if lim.Cores == 0 {
		lim.Cores = 8
	}
	if lim.ClosureBytes == 0 {
		lim.ClosureBytes = 20 << 30
	}
	if pct, ok := m.storeUsedPct(); ok && pct >= m.cfg.StoreHighPct {
		return nil, errf(CodeInsufficientCapacity, "host store full")
	}
	log := m.d.Log.With("component", "hostd", "project_id", c.ProjectId, "command_id", commandID)
	log.Info("build start", "event", "build_start", "revision_id", c.RevisionId)
	start := m.d.Now()
	if m.d.Metrics != nil {
		m.d.Metrics.BuildsRunning.Inc()
		defer m.d.Metrics.BuildsRunning.Dec()
	}
	m.mu.Lock()
	m.buildRun++
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.buildRun--
		m.mu.Unlock()
	}()
	var seq uint64
	res, err := m.d.Nix.Build(ctx, nixbuild.Request{
		ProjectID: c.ProjectId, RevisionID: c.RevisionId, Fragment: c.Fragment, BaseRef: c.BaseRef,
		Limits: nixbuild.Limits{EvalS: lim.EvalS, BuildS: lim.BuildS, Cores: lim.Cores, ClosureBytes: lim.ClosureBytes},
	}, func(line string) {
		seq++
		m.d.Emit.BuildLog(commandID, seq, line)
	})
	dur := m.d.Now().Sub(start)
	if err != nil {
		var ne *nixbuild.Error
		if errors.As(err, &ne) {
			log.Warn("build failed", "event", "build_fail", "code", ne.Code, "duration_ms", dur.Milliseconds())
			if m.d.Metrics != nil {
				m.d.Metrics.BuildDuration.WithLabelValues(ne.Code).Observe(dur.Seconds())
			}
			return nil, &Error{Code: ne.Code, Message: ne.Message, FragmentLine: ne.FragmentLine}
		}
		log.Error("build failed", "event", "build_fail", "code", CodeInternal, "duration_ms", dur.Milliseconds())
		if m.d.Metrics != nil {
			m.d.Metrics.BuildDuration.WithLabelValues("internal").Observe(dur.Seconds())
		}
		return nil, errf(CodeInternal, "build: %v", err)
	}
	if m.d.Metrics != nil {
		m.d.Metrics.BuildDuration.WithLabelValues("ok").Observe(dur.Seconds())
	}
	if res.CacheUnreachable {
		m.Warn("cache_unreachable", "substituter unreachable during build of revision "+c.RevisionId+"; built from source")
	}
	log.Info("build done", "event", "build_done", "duration_ms", dur.Milliseconds(), "closure_bytes", res.ClosureBytes)
	return &hostdv1.BuildResult{SystemClosure: res.SystemClosure, ClosureBytes: res.ClosureBytes, KernelChanged: m.kernelChanged(c.ProjectId, res)}, nil
}

// kernelChanged compares the built kernel and initrd with what the
// project's guest last booted or applied.
func (m *Manager) kernelChanged(projectID string, res *nixbuild.Result) bool {
	gs, err := m.d.State.ListGuests()
	if err != nil {
		return false
	}
	for _, g := range gs {
		if g.ProjectID != projectID || g.Kernel == "" {
			continue
		}
		return g.Kernel != res.Kernel || g.Initrd != res.Initrd
	}
	return false
}

func (m *Manager) apply(ctx context.Context, c *hostdv1.ApplyConfig) (*hostdv1.ApplyResult, *Error) {
	g, gerr := m.getGuest(c.GuestId)
	if gerr != nil {
		return nil, gerr
	}
	if c.SystemClosure == "" {
		return nil, errf(CodeInvalidArgument, "system_closure required")
	}
	if ok, err := m.d.Nix.PathExists(ctx, c.SystemClosure); err != nil {
		return nil, errf(CodeInternal, "nix path-info: %v", err)
	} else if !ok {
		return nil, errf(CodeNotFound, "system closure %s is not in the host store", c.SystemClosure)
	}
	if g.State != StateRunning {
		// A stopped guest only needs the root moved; the next start boots it.
		if err := m.adoptClosure(g, c.SystemClosure); err != nil {
			return nil, err
		}
		return &hostdv1.ApplyResult{}, nil
	}
	sess, serr := m.session(g.GuestID)
	if serr != nil {
		return nil, serr
	}
	reg, derr := m.d.Nix.DumpDB(ctx, c.SystemClosure)
	if derr != nil {
		return nil, errf(CodeInternal, "nix-store --dump-db: %v", derr)
	}
	sw, err := sess.Switch(ctx, c.SystemClosure, false, reg)
	if err != nil {
		if re, ok := vsockclient.IsRemote(err); ok {
			out := ""
			if sw != nil {
				out = string(sw.Output)
			}
			return nil, errf(CodeInternal, "switch failed: %s: %s\n\n%s", re.Code, re.Message, out)
		}
		return nil, errf(CodeGuestUnresponsive, "guestd Switch: %v", err)
	}
	if sw.NeedsReboot && !c.ForceReboot {
		m.log(g).Info("apply needs reboot", "event", "switch_done", "reboot_required", true)
		return &hostdv1.ApplyResult{RebootRequired: true}, nil
	}
	if sw.NeedsReboot {
		// force_reboot: snapshot, stop, adopt the closure, start.
		if _, err := m.snapshotGuest(ctx, g, "stop"); err != nil {
			return nil, err
		}
		if err := m.stopGuest(ctx, g, 0); err != nil {
			return nil, err
		}
		if err := m.adoptClosure(g, c.SystemClosure); err != nil {
			return nil, err
		}
		if err := m.boot(ctx, g, stepRunner); err != nil {
			return nil, err
		}
		m.log(g).Info("apply rebooted", "event", "switch_done", "rebooted", true)
		return &hostdv1.ApplyResult{Rebooted: true}, nil
	}
	if err := m.adoptClosure(g, c.SystemClosure); err != nil {
		return nil, err
	}
	m.log(g).Info("apply switched", "event", "switch_done", "rebooted", false)
	return &hostdv1.ApplyResult{}, nil
}

// adoptClosure moves the guest's GC root and records the closure's kernel
// and initrd.
func (m *Manager) adoptClosure(g *state.Guest, closure string) *Error {
	if err := m.d.Roots.Set(g.GuestID, closure); err != nil {
		return errf(CodeInternal, "gcroot: %v", err)
	}
	g.SystemClosure = closure
	if info, err := nixbuild.ClosureInfo(closure); err == nil {
		g.Kernel, g.Initrd = info.Kernel, info.Initrd
	}
	if err := m.d.State.PutGuest(g); err != nil {
		return errf(CodeInternal, "state write: %v", err)
	}
	m.writeGuestJSON(g)
	return nil
}

var _ = time.Second
