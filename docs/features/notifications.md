# Notifications

When an agent finishes or gets stuck, the user hears about it on their phone
or in their inbox, whichever agent it was. Agents' own notification features
keep working on top.

## What the user sees

```
$ repose notify set ntfy https://ntfy.sh/heracraft-repose-8f3a
Test notification sent. Check your phone.

$ repose notify set email on
```

On the phone (ntfy):

```
repose · todo-app
claude finished: "Added auth flow, 14 tests green, committed 3f9e2a1"
```

```
repose · todo-app
codex needs input: "Should I drop the legacy sessions table?"
```

In `repose status`:

```
todo-app   large   running   claude: needs_input (2m ago)   last: "Should I drop..."
```

## Behaviour that must hold

Events (see agents.md for how each agent produces them):

- Kinds: `completed`, `needs_input`, `error`, plus the platform-originated
  `billing_stopped`, `base_updated`, `base_update_failed`, `snapshot_failed`
  and `host_moved`. Each carries the agent name (agent kinds only), the
  tmux window, a summary of at most 1 KB, and a timestamp.
- The summary is what the agent's hook provided, truncated. It may include
  the agent's own last message. It never includes the prompt the user typed
  or terminal contents beyond what the hook payload carries.
- Heuristic events (pane idle for agents without hooks) are labelled as
  such in the message: `gemini went idle` rather than `gemini finished`.
  As shipped, the heuristic never reads pane *text* at all — only whether
  the pane's process tree has used CPU recently — which is stronger than
  "never sent", not weaker (DECISIONS I-49).
- An event reaches the API within 10 seconds of the hook (90 seconds for
  heuristics), and the outbox worker picks up undelivered rows every 2
  seconds. Delivery results are stored per channel on the event row.
- Duplicate suppression: the same `(project, agent, kind)` within 60
  seconds is one event, with a later summary appended to the first rather
  than dropped. A Claude `Stop` hook that fires twice for one turn is the
  reason.
- Rate cap: at most 30 notifications per project per hour; past that, one
  message says notifications are paused for this project until the top of
  the hour, and events are still recorded (never dropped, just not
  delivered past the cap).

Channels (`workstreams/13-notifications.md` §5.6):

- Email through Resend, to the account's email, one message per event,
  subject `[repose] <project>: <agent> <verb>` for agent events (e.g.
  `[repose] todo-app: claude finished`) or a dedicated line for platform
  events (today: `Your guests were stopped for non-payment`). The body
  carries the summary, a `repose attach` hint, a dashboard link, and — once
  the platform's signing key exists, which it does from the api's first
  start — a one-click unsubscribe link that turns email off with no login
  required. On by default at signup; no time-based default change.
- ntfy: the user sets any ntfy-compatible URL, including a self-hosted
  server; the platform POSTs the message with a title, a priority (higher
  for `needs_input` and failures), and a `click` URL pointing at the
  project in the dashboard. A test notification is sent on set (`repose
  notify test` / the dashboard's test button, `POST /me/notify-test`).
  Topic URLs are stored as configuration, not secrets, but never logged.
- Both can be on. Neither is required. Turning one off deletes its
  already-queued deliveries rather than sending one more batch to a
  channel the user just disabled.

Agents' own features are untouched: Claude Code Remote Control works from a
guest when the user logged in with a subscription inside it; Claude channels
(Telegram, Discord) work if the user configures them. The platform does not
proxy or intercept these.

Dashboard and CLI:

- `repose status` shows the last event per agent window.
- `repose events`, its own command, lists
  the last 50 events with timestamps.
- The dashboard project page shows the event stream and delivery status per
  channel, so a user who got nothing can see whether the event happened and
  whether the delivery failed.

## Depends on

Workstreams 13 (delivery, dedupe, rate cap, Resend, ntfy, unsubscribe), 04
(hook socket, AgentEvent, the tmux-idle watcher), 03 (Event forwarding), 05
(ingest, events routes, `PATCH /me` notify settings, the ops engine's
platform-event hook), 07 (`notify` and `events` commands), 08 (settings and
event stream pages), 02 (`repose-hook` and wrappers), 09 (`billing_stopped`'s
producer, not yet built — see `DECISIONS.md` I-16 and I-49).

## Deferred

Telegram and Discord webhooks. Web push from the dashboard. A platform
mobile app. Per-project channel overrides. Digest mode.
