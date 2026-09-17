# 13 · notifications

## 1. Goal

When an agent the user left running finishes or gets stuck, the user finds
out on their phone within a minute, whichever agent it was. One event
pipeline from the guest to the user's channels, with the same shape for
every agent, so `factory status` and the dashboard show one timeline.

## 2. Scope: builds

- Hook side in the guest: `factory-hook` (Go, part of `cmd/guestd` as a
  subcommand or a tiny separate binary in the same package), the
  per-agent hook configuration that the agent wrappers install
  (02-guest-base installs the wrappers; this workstream defines what they
  install), and the tmux-idle heuristic for agents without hooks.
- guestd's `AgentEvent` and `AgentState` notifications (04-guestd
  implements the transport; this workstream defines when they fire and
  what they carry).
- api side: `POST`-free ingest from the gRPC `Event` message into `events`,
  the outbox, delivery workers for email (Resend) and ntfy, dedupe, retries,
  user settings, `POST /me/notify-test`.
- Templates for email and ntfy.
- Platform-originated events that reuse the pipeline: `billing_stopped`,
  `base_updated`, `snapshot_failed`, `host_moved` (restore onto a new host).
- What `factory status` and the dashboard events card display (07 and 08
  render; this workstream defines the fields and wording).

## 3. Scope: does not build

- vsock transport and the unix hook socket (04-guestd).
- Wrapper packaging in Nix (02-guest-base; this doc supplies the hook
  config each wrapper writes).
- Telegram and Discord webhooks (DESIGN §18; the delivery interface is
  designed so they are one file each later).
- Anything with Claude Code Remote Control or channels: those belong to the
  user's own Claude login and the platform does not touch them.
- Email for auth or billing receipts (Logto and Stripe send their own).

## 4. Interfaces

Owns: event shapes (`kind`, `summary` rules), the hook JSON on
`/run/factory/hooks.sock`, the `events.delivered` JSON, the
`notify_email` and `ntfy_url` semantics on `users`, `POST /me/notify-test`
(added to `interfaces/api.md`).

Consumes: `interfaces/vsock-guestd.md` (`AgentEvent`, `AgentState`),
`interfaces/grpc-hostd.md` (`Event.agent_event`), `interfaces/api.md`
(`GET /events`, `PATCH /me`), `interfaces/guest-conventions.md` (window
names, wrapper behaviour).

## 5. Design detail

### 5.1 Event shape

```
kind      completed | needs_input | error | billing_stopped | base_updated |
          snapshot_failed | host_moved
agent     claude | opencode | codex | gemini | pi | null (platform events)
window    tmux window name, e.g. claude-2, or null
summary   ≤ 1024 bytes, plain text, first line ≤ 120 chars used as the title
project   id (slug and name joined for display)
ts        RFC 3339, from the guest clock, replaced by the api's clock if
          skewed more than 5 minutes
```

`summary` is what the agent's hook payload gives (Claude's `message` for
Notification, the last assistant line for Stop, truncated), never the
prompt and never terminal contents beyond what the hook payload itself
carries. The privacy boundary in `DESIGN.md` §15 applies: if a hook payload
contains a transcript path, it is not opened.

### 5.2 Hook socket protocol

`POST http://unix/run/factory/hooks.sock/v1/event` with JSON
`{"agent": "claude", "window": "claude", "kind": "completed", "summary":
"..."}`. Response 202 always, even on validation failure (logged), so a
hook never blocks an agent. `factory-hook` is what agents run; it reads the
agent's native payload from stdin, maps it, resolves `window` from
`$TMUX_PANE` via `tmux display -p -t $TMUX_PANE '#{window_name}'`, and
POSTs. It exits 0 in every case, including when the socket is missing.

### 5.3 Per-agent mechanisms

| Agent | Mechanism | `completed` | `needs_input` | `error` |
|---|---|---|---|---|
| Claude Code | `~/.claude/settings.json` hooks written by the wrapper if absent: `Notification` with matchers `agent_completed`, `agent_needs_input`, `permission_prompt`, `idle_prompt`; `Stop`; `StopFailure`. Command `factory-hook claude`. | `Stop` and `Notification:agent_completed` (deduped, 5.5) | `Notification:agent_needs_input`, `permission_prompt`, `idle_prompt` | `StopFailure` |
| Codex CLI | `~/.codex/config.toml` `notify = ["factory-hook", "codex"]` (Codex calls it with a JSON arg on `agent-turn-complete`) | `agent-turn-complete` | tmux-idle heuristic (5.4) | process exit non-zero while window present (guestd) |
| opencode | plugin file `~/.config/opencode/plugin/factory.js` written by the wrapper, subscribing to `session.idle` and `permission.asked` events and calling `factory-hook opencode` | `session.idle` | `permission.asked` | tmux-idle with error pattern (5.4) |
| Gemini CLI | no stable hook API at time of writing; tmux-idle heuristic only | idle after activity | idle with a prompt marker on the last line | non-zero exit |
| pi | `~/.pi/agent/hooks/` if present in the packaged version, else tmux-idle | as available | as available | non-zero exit |

The wrapper for each agent writes its hook config only if the key is absent
(a user's own hooks are preserved and `factory-hook` is appended, never
replacing). The exact file edits are in `interfaces/guest-conventions.md`
under agent wrappers; the mapping from native payload to `kind` lives in
`internal/hooks/<agent>.go` with a fixture of each native payload.

Where a mechanism above says "at time of writing", the implementer checks
the agent's current docs and records the finding in `DECISIONS.md` as an
implementation entry; a heuristic is never left in place if a hook exists.

### 5.4 tmux-idle heuristic (guestd)

For each window whose name is an agent name, every 5 seconds guestd reads
`#{pane_current_command}` and the pane's last 3 lines (`tmux capture-pane
-p -S -3`, kept in memory only, never logged or sent). State machine per
window:

- `working`: output changed in the last 30 s.
- `idle`: no change for 30 s. Transition working → idle emits
  `AgentState idle`; if the window has emitted no `completed` via a real
  hook in the last 60 s, it emits a synthetic `completed` with `summary =
  "<agent> has been idle for 30s"`.
- `needs_input`: idle and the last line matches the agent's prompt pattern
  (a table in `internal/hooks/patterns.go`: Claude `❯`, Codex `›`, Gemini
  `>`, opencode `>`, pi `❯`, plus `[y/N]`, `(y/n)`, `Allow?`). Emits
  `needs_input` once per idle period.
- window gone: if `pane_dead` or the window closed with a non-zero exit,
  emit `error` with `summary = "<agent> exited with status N"`.

The heuristic is suppressed for an agent that has a real hook (Claude,
Codex, opencode) except for the exit case, so the pipeline does not double
up. The heuristic is the reason `AgentState` exists separately from
`AgentEvent`: state feeds `factory status` and the samples; events feed
notifications.

### 5.5 Ingest, dedupe, outbox

api receives `Event.agent_event` on the host stream, acks by `event_id`,
inserts `events` with `delivered = {}`. Dedupe: same `(project, agent,
window, kind)` within 60 seconds collapses to the first, with the later
summary appended if different (Claude fires both `Stop` and
`agent_completed`). An `events_outbox` table (`event_id, channel, attempts,
next_at, last_error`) is filled at insert with one row per enabled channel.
A worker polls the outbox every 2 seconds (`for update skip locked`),
delivers, and writes `events.delivered[channel] = ts` or `error`. Retries: 5
attempts at 10 s, 1 m, 5 m, 30 m, 2 h; after that `delivered[channel] =
"failed: <reason>"` and a `notify_delivery_failed` metric. Rate limit per
user per channel: 30 per hour, beyond that events are stored and a single
"30+ events in the last hour, see the dashboard" message is sent.

### 5.6 Channels

- **email** (Resend, `internal/notify/email.go`): from
  `factory <notify@factory.herakraft.co>`, subject `[factory] todo-app:
  claude finished`, body: title, summary, `factory attach` hint, dashboard
  link, unsubscribe link (sets `notify_email = false` through a signed
  token route `GET /notify/unsubscribe?token=`). Enabled by default at
  signup. Platform events use their own subjects (`Your guests were stopped
  for non-payment`).
- **ntfy** (`internal/notify/ntfy.go`): `POST <ntfy_url>` with headers
  `Title`, `Priority` (5 for `needs_input`, 3 otherwise), `Tags`
  (`white_check_mark`, `question`, `x`), `Click` (dashboard project URL),
  body = summary. The user pastes any ntfy-compatible URL including
  self-hosted or `ntfy.sh/<topic>`; the URL is stored as is and never
  logged.
- `POST /me/notify-test` sends a `completed` event with summary `This is a
  test from factory` through the enabled channels and returns the per-channel
  result so the settings page can show it.

Adding Telegram or Discord is a new file implementing
`Channel{Send(ctx, user, event) error}` and a user setting; nothing else
changes.

### 5.7 Display

`factory status` shows the last event (`last event 12m ago: claude
completed "ran tests, 3 failures fixed"`) and `AgentState` per window
(`claude: working`). The dashboard events card lists the last 50 with
delivery status icons per channel. `GET /events?since=` is the only read
path.

### 5.8 Metrics

`factory_notify_events_total{kind,agent,source=hook|heuristic}`,
`factory_notify_delivered_total{channel,result}`,
`factory_notify_outbox_depth`, `factory_notify_delivery_latency_seconds`
(event ts → delivered ts), `factory_notify_dedupe_total`.

## 6. Failure modes

| Situation | Outcome |
|---|---|
| Hook socket missing or guestd down | `factory-hook` exits 0 silently, guestd logs on next start; the tmux-idle heuristic still runs once guestd is back |
| Agent payload unparseable | `factory-hook` sends `kind = completed`, `summary = "<agent> event (unparsed)"`; fixture added |
| Both hook and heuristic fire | dedupe window collapses them |
| Resend down | retries per 5.5, `failed` after 5, metric and alert if > 5 percent failed in 10 minutes |
| ntfy URL invalid or 4xx | first failure marks `delivered.ntfy = "failed: 404"` with no retry (4xx), the settings page shows a warning banner |
| User disables email mid-retry | outbox rows for that channel are deleted at the setting change |
| Event storm (agent looping) | rate limit per 5.5, one digest message |
| Clock skew from guest | api replaces `ts` and records `skew_seconds` in the event |
| Summary contains secrets (agent echoed a token) | not detectable; mitigated by summaries being ≤ 1 KB and never the transcript; documented in `SECURITY.md` as accepted |

## 7. Testing

- Unit: payload mapping fixtures for each agent's native hook JSON, the
  idle state machine with a fake clock, dedupe, outbox scheduling, rate
  limit, templates (golden).
- Integration: real Postgres, outbox worker with a fake Resend and a local
  ntfy container, retries with a fake clock, `notify-test` route.
- Guest-level: on a real guest, run each agent, trigger a completion and a
  permission prompt, observe the event arrive in `events` with the right
  kind and source, and the phone notification within 60 s. Recorded per
  agent in `STATUS.md`.
- Wrapper: a test that runs each wrapper in a temp `$HOME` and asserts the
  hook config it writes, and that it preserves pre-existing user hooks.

## 8. Rollback

Migrations down for `events_outbox`. Disabling delivery is
`NOTIFY_CHANNELS=` empty, which keeps ingesting events (they still show in
status) and delivers nothing. Wrappers writing hook config are idempotent
and reversible by deleting the `factory-hook` entries; a `factory-hook
uninstall` subcommand does that for every agent.

## 9. Checklist

- [ ] Hook socket protocol implemented; `factory-hook` exits 0 in every
      case including a missing socket. Evidence: test that removes the
      socket.
- [ ] Every agent row in 5.3 has a fixture of the native payload and a
      mapping test; "at time of writing" rows resolved and recorded in
      `DECISIONS.md`. Evidence: fixtures directory listing and the
      decision entry.
- [ ] Wrapper hook config for each agent is written when absent, appended
      when the user has hooks, never replaces. Evidence: wrapper test
      output with a pre-existing hook.
- [ ] tmux-idle state machine covers working, idle, needs_input, exit;
      suppressed for hooked agents except exit. Evidence: unit test with
      fake clock and fake pane content.
- [ ] Pane contents are never logged, stored or sent. Evidence: reviewer
      grep of guestd for `capture-pane` uses and their sinks.
- [ ] Dedupe collapses Claude's `Stop` + `agent_completed`. Evidence: test.
- [ ] Outbox retries at the stated schedule, marks failed after 5, deletes
      rows when a channel is disabled. Evidence: fake-clock test.
- [ ] Rate limit sends one digest beyond 30 per hour. Evidence: test.
- [ ] Email template renders with unsubscribe link that works. Evidence:
      golden test plus a real email received.
- [ ] ntfy delivery with priority and tags per kind; 4xx not retried.
      Evidence: test against a local ntfy.
- [ ] `POST /me/notify-test` returns per-channel results and the dashboard
      shows them. Evidence: transcript.
- [ ] Platform events (`billing_stopped`, `base_updated`,
      `snapshot_failed`, `host_moved`) flow through the same pipeline.
      Evidence: test per kind.
- [ ] Real guest: each of the five agents produces a `completed` and, where
      the mechanism supports it, a `needs_input`, delivered to a phone
      within 60 s. Evidence: `STATUS.md` line per agent with the event ids.
- [ ] `factory status` and the dashboard show last event and agent state.
      Evidence: screenshot and CLI output.
- [ ] Metrics in 5.8 exist. Evidence: `/metrics` scrape.
- [ ] `features/notifications.md` matches. Evidence: implementer re-read.
- [ ] `ops/RUNBOOK.md` has: no notifications arriving (check outbox depth,
      guestd_ok, hook config), ntfy failing, Resend failing. Evidence:
      entries exist.
