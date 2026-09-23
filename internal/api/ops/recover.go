package ops

import (
	"context"
	"strings"

	"github.com/heracraft/repose/internal/api/store"
)

// Codes the recovery paths react to (grpc-hostd.md error codes).
const (
	codeGuestUnresponsive = "guest_unresponsive"
	codeNotFound          = "not_found"
)

// PlanRestart is a start that reboots the guest instead of trusting what
// runs (I-157): stop the unit without a snapshot (guestd may be dead, so
// nothing can freeze), move the stopped guest onto the newest built
// revision when there is one (hostd adopts the closure of a stopped guest
// without asking guestd), then boot it. It is what `repose start` enqueues
// for a project in `error` or one whose guestd stopped answering.
func PlanRestart(pendingRevision bool) []string {
	if pendingRevision {
		return []string{PhaseStopGuest, PhaseApplyConfig, PhaseStartGuest}
	}
	return []string{PhaseStopGuest, PhaseStartGuest}
}

// RestartParams are the params a PlanRestart op carries.
func RestartParams() map[string]any { return map[string]any{"snapshot": false, "restart": true} }

func isRestart(op *store.Op) bool {
	v, _ := op.Params["restart"].(bool)
	return v
}

// recoverFrom decides whether a failed phase has a path that still ends
// the op as the user asked (I-156, I-157, I-158). It rewrites the op's remaining
// phases (or skips the failed one) and returns true; the next tick sends
// the new phase. It returns false when the failure is the op's result.
//
// Each recovery happens at most once per op ("recovered" in params), so a
// guest that fails the same way after the fallback ends the op in error
// instead of looping.
func (e *Engine) recoverFrom(ctx context.Context, op *store.Op, code string) bool {
	ph := currentPhase(op)
	all := phases(op)
	recovered, _ := op.Params["recovered"].(string)
	var next []string
	skip := false
	switch {
	// A destroy whose guest is no longer on its host has nothing left to
	// stop, snapshot or delete there: the host's answer is the end state
	// the user asked for.
	case op.Kind == KindDestroy && code == codeNotFound &&
		(ph == PhaseStopGuest || ph == PhaseSnapshot || ph == PhaseDestroyGuest):
		next = []string{}

	// The final snapshot of a destroy could not be taken even after the
	// guest was stopped: skip it, say so, and keep the older snapshots
	// (their 30-day expiry is set by markDestroyed).
	case op.Kind == KindDestroy && code == codeGuestUnresponsive && ph == PhaseSnapshot && recovered != "":
		e.notifyPlatform(ctx, *op.ProjectID, "snapshot_failed",
			"the final snapshot before destroy was skipped because the environment could not be read; the newest earlier snapshot is kept for 30 days")
		skip = true

	case recovered != "" || code != codeGuestUnresponsive:
		return false

	// guestd is dead: nothing can freeze the filesystem, so a running
	// guest is stopped first (hostd falls back to the hypervisor and then
	// to killing the unit) and the snapshot is taken of the stopped
	// volume, which is crash-consistent at worst.
	case op.Kind == KindDestroy && (ph == PhaseSnapshot || ph == PhaseStopGuest):
		next = []string{PhaseStopGuest, PhaseSnapshot, PhaseDestroyGuest}
		op.Params["snapshot"] = false
		op.Params["reason"] = "stop"
	case op.Kind == KindStop && ph == PhaseStopGuest:
		snap := true
		if v, ok := op.Params["snapshot"].(bool); ok {
			snap = v
		}
		if !snap {
			return false // the stop itself needs no guestd; nothing to fall back to
		}
		next = []string{PhaseStopGuest, PhaseSnapshot}
		op.Params["snapshot"] = false
		op.Params["reason"] = "stop"
	// A start whose switch lost guestd (I-143's strand): reboot the guest
	// onto the revision the switch was applying.
	case op.Kind == KindStart && ph == PhaseApplyConfig:
		next = PlanRestart(true)
		for k, v := range RestartParams() {
			op.Params[k] = v
		}
	default:
		return false
	}
	op.Params["recovered"] = ph + ":" + code
	var ph2 []any
	for _, p := range all[:op.Step] {
		ph2 = append(ph2, p)
	}
	if skip {
		for _, p := range all[op.Step:] {
			ph2 = append(ph2, p)
		}
		op.Step++
	} else {
		for _, p := range next {
			ph2 = append(ph2, p)
		}
	}
	if ph2 == nil {
		ph2 = []any{}
	}
	op.Params["phases"] = ph2
	if op.CommandID != nil {
		e.logs.Unbind(op.CommandID.String())
	}
	op.CommandID, op.CommandResult, op.SentAt = nil, nil, nil
	if _, err := e.pool.Exec(ctx, "update ops set params = $2, step = $3, command_id = null, command_result = null, sent_at = null where id = $1", op.ID, op.Params, op.Step); err != nil {
		e.log.Error("op recovery record", "event", "op_fail", "op_id", op.ID.String(), "err", err.Error())
		return false
	}
	action := "replanned"
	if skip {
		action = "skipped"
	}
	e.log.Warn("op phase failed; recovering", "event", "op_recover", "op_id", op.ID.String(), "kind", op.Kind,
		"phase", ph, "code", code, "action", action, "phases", strings.Join(phases(op)[op.Step:], ","))
	e.Kick()
	return true
}

// humanError turns a host's error into the sentence a user reads in
// op.error.message and projects.last_error (I-159). The code is kept; the
// host's own wording, which names internal ids, moves to detail.
func humanError(kind, phase, code, msg string) (string, bool) {
	switch code {
	case codeGuestUnresponsive:
		if phase == PhaseCreateGuest || phase == PhaseStartGuest || phase == PhaseRestore {
			return "the environment booted but its agent (guestd) never answered; `repose logs --kind console` shows the boot, and `repose start` tries again", true
		}
		switch kind {
		case KindResize:
			return "the disk grew, but the environment's agent (guestd) stopped answering, so the filesystem inside did not; `repose start` restarts it, then run the resize again", true
		case KindSnapshot:
			return "the environment's agent (guestd) stopped answering, so the filesystem could not be frozen for a snapshot; `repose start` restarts it", true
		}
		return "the environment's agent (guestd) stopped answering; `repose start` restarts it", true
	case "host_unreachable":
		return "the host running this project stopped answering; the operation can be run again once the host is back", true
	case "insufficient_capacity":
		return "the host has no room for this project right now; try again in a few minutes", true
	}
	return msg, false
}
