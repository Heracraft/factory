package obs

import "sort"

// Every log event named in docs/workstreams/10-observability.md §5, as
// constants so a call site cannot misspell one and so the coverage test can
// find the ones nobody emits. "Events are intentional, not fmt.Sprintf
// residue": a component may emit events beyond this list (hostd's own §5.14
// list is longer), but it must emit all of its own.
const (
	// hostd
	EventGuestCreate      = "guest_create"
	EventGuestStart       = "guest_start"
	EventGuestStop        = "guest_stop"
	EventGuestDestroy     = "guest_destroy"
	EventGuestState       = "guest_state"
	EventBuildStart       = "build_start"
	EventBuildDone        = "build_done"
	EventBuildFail        = "build_fail"
	EventSwitchDone       = "switch_done"
	EventSnapshotStart    = "snapshot_start"
	EventSnapshotDone     = "snapshot_done"
	EventSnapshotFail     = "snapshot_fail"
	EventStreamConnect    = "stream_connect"
	EventStreamDisconnect = "stream_disconnect"
	EventGuestdLost       = "guestd_lost"
	EventGuestdRegained   = "guestd_regained"
	EventPoolWarning      = "pool_warning"
	EventStoreWarning     = "store_warning"

	// guestd
	EventReady          = "ready"
	EventFreeze         = "freeze"
	EventThaw           = "thaw"
	EventFreezeTimeout  = "freeze_timeout"
	EventSwitch         = "switch"
	EventAgentEvent     = "agent_event"
	EventAgentState     = "agent_state"
	EventHookBadPayload = "hook_bad_payload"
	EventGrowFs         = "grow_fs"
	EventWriteSecrets   = "write_secrets"
	EventSetPrincipals  = "set_principals"
	EventSetupProject   = "setup_project"
	EventSample         = "sample"
	EventExec           = "exec"
	EventShutdown       = "shutdown"
	EventWarning        = "warning"

	// api
	EventRequest       = "request"
	EventCertIssue     = "cert_issue"
	EventCertRevoke    = "cert_revoke"
	EventSchedule      = "schedule"
	EventScheduleFail  = "schedule_fail"
	EventCommandSend   = "command_send"
	EventCommandResult = "command_result"
	EventRollupDone    = "rollup_done"
	EventStripeWebhook = "stripe_webhook"
	EventNotifySend    = "notify_send"
	EventNotifyFail    = "notify_fail"
	EventAdminAction   = "admin_action"
	// EventPartitionDropFail is the §6 failure mode: the meter_samples or
	// proc_samples partition drop did not run, so disk grows and nothing
	// else breaks.
	EventPartitionDropFail = "partition_drop_fail"

	// gateway
	EventSessionOpen  = "session_open"
	EventSessionClose = "session_close"
	EventAuthFail     = "auth_fail"
	EventRouteFail    = "route_fail"
	EventDialFail     = "dial_fail"
)

// RequiredEvents is the per-component list §5 calls "the events each
// component must emit". The coverage test in events_test.go checks a
// component's list against the source of the repository.
var RequiredEvents = map[Component][]string{
	ComponentHostd: {
		EventGuestCreate, EventGuestStart, EventGuestStop, EventGuestDestroy,
		EventGuestState, EventBuildStart, EventBuildDone, EventBuildFail,
		EventSwitchDone, EventSnapshotStart, EventSnapshotDone, EventSnapshotFail,
		EventStreamConnect, EventStreamDisconnect, EventGuestdLost,
		EventGuestdRegained, EventPoolWarning, EventStoreWarning,
	},
	ComponentGuestd: {
		EventReady, EventFreeze, EventThaw, EventFreezeTimeout, EventSwitch,
		EventAgentEvent, EventAgentState, EventHookBadPayload, EventGrowFs,
		EventWriteSecrets, EventSetPrincipals, EventSetupProject, EventSample,
		EventExec, EventShutdown, EventWarning,
	},
	ComponentAPI: {
		EventRequest, EventCertIssue, EventCertRevoke, EventSchedule,
		EventScheduleFail, EventCommandSend, EventCommandResult, EventRollupDone,
		EventStripeWebhook, EventNotifySend, EventNotifyFail, EventPartitionDropFail,
	},
	ComponentGateway: {
		EventSessionOpen, EventSessionClose, EventAuthFail, EventRouteFail,
		EventDialFail,
	},
	// The CLI logs only to ~/.config/repose/cli.log at debug level with
	// --verbose and ships nothing from a laptop, so it has no required
	// events. hostdev and repose-hook are dev and helper binaries.
	//
	// admin_action is the admin CLI's, not the api's: §5 put it on the api's
	// list expecting admin actions to arrive as api calls, and workstream 05
	// built repose-admin against Postgres directly (DECISIONS I-59), so the
	// line is written where the audit_log row is.
	ComponentCLI:     {},
	ComponentAdmin:   {EventAdminAction},
	ComponentHostdev: {},
	ComponentHook:    {},
}

// PendingEvents are events in §5 whose producer does not exist yet, with the
// workstream that owes each one. The coverage test reports them instead of
// failing, so that a missing producer is visible without blocking the
// workstreams that are built.
var PendingEvents = map[string]string{
	// The Stripe webhook route is workstream 09 (billing); the api starts
	// without STRIPE_* set and its billing routes return 503 (DECISIONS
	// I-16), so there is nothing to log yet.
	EventStripeWebhook: "workstream 09 (billing): no webhook route exists yet",
}

// AllRequiredEvents is every event in RequiredEvents, sorted and deduped.
func AllRequiredEvents() []string {
	seen := map[string]bool{}
	var out []string
	for _, evs := range RequiredEvents {
		for _, e := range evs {
			if !seen[e] {
				seen[e] = true
				out = append(out, e)
			}
		}
	}
	sort.Strings(out)
	return out
}
