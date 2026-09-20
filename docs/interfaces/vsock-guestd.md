# vsock: hostd ⇄ guestd

Inside a guest, `guestd` listens on vsock port 5000. hostd connects once per
guest (CID assigned at create) and keeps the connection; guestd accepts one
connection at a time and drops the older one. Framing: length-prefixed
protobuf (`proto/repose/guestd/v1/guestd.proto`), request/response with
`request_id`, plus unsolicited `Notify` messages from guestd.

No network listener exists in guestd. A fake for hosts without vsock (dev
machines) uses a Unix socket at `/run/repose/guestd.sock` with the same
framing; hostd picks by flag.

## Requests (hostd → guestd)

| Request | Fields | Response | Notes |
|---|---|---|---|
| `Ping` | | version, uptime_s, boot_id | also the readiness check after boot; `version` is the guestd *protocol* version (`1` today), which is what hostd compares before sending a request a guest may not know |
| `Freeze` | | | `fsfreeze -f /`; guestd sets a 10 s watchdog that thaws if no `Thaw` arrives |
| `Thaw` | | | |
| `Switch` | system_closure, force_reboot, registration (bytes, I-55) | rebooted (bool), needs_reboot (bool), output (capped 32 KB) | loads `registration` (a `nix-store --dump-db` of the closure) into the guest's nix database when present, then runs `<closure>/bin/switch-to-configuration switch`; if the closure's kernel or initrd differ from the running one, responds `needs_reboot=true` and does nothing unless `force_reboot` |
| `RegisterPaths` | registration (bytes) | | `nix-store --load-db` of a `nix-store --dump-db` listing, so paths the guest sees through the shared store are valid in its own database; hostd sends the booted closure's listing right after `Ready`, before `WriteSecrets`. Writes `/run/repose/paths-registered` (DECISIONS I-55) |
| `GrowFs` | | new_bytes | `resize2fs` after the host grew the volume |
| `WriteSecrets` | list {name, bytes} | | writes `/run/repose/secrets/<name>` 0400 dev on tmpfs, rewrites `/run/repose/secrets.env`. The list is the **whole set**: a secret present in the guest and absent from the list is removed, which is how `repose secrets rm` reaches a running guest. The three reserved names of DECISIONS I-10 are never removed this way. Validation is per request: one bad name or oversized value rejects the batch and writes nothing (DECISIONS I-30) |
| `SetPrincipals` | list | | writes `/etc/ssh/principals/dev`, reloads sshd |
| `SetupProject` | project_slug, remote_url, tz, lang | | creates tmux session named slug, `/home/dev/<slug>`, git init if empty, writes `/home/dev/.repose/project.json` |
| `Sample` | | GuestSignals + repeated ProcSample (shapes in grpc-hostd.md) + partial (bool) | hostd calls every 60 s. guestd walks `/proc` on the call and serves the tmux and Docker signals from a 5 s background refresh, so a sample costs under 20 ms and never forks (DECISIONS I-31). `partial` is set when a signal is missing rather than zero |
| `Exec` | argv, timeout_s, as_user | exit_code, stdout, stderr | operator only; hostd audits every call |
| `Shutdown` | timeout_s | | `systemctl poweroff` after flushing |

## Notifications (guestd → hostd)

| Notify | Fields | Origin |
|---|---|---|
| `Ready` | boot_id | after network up and sshd listening |
| `AgentEvent` | agent, tmux_window, kind (completed\|needs_input\|error), summary | agent hooks via the unix socket `/run/repose/hooks.sock` |
| `AgentState` | agent, tmux_window, state | on change, debounced 5 s |
| `Warning` | kind, detail | kinds: `disk_high` (over 90 percent), `inotify_exhausted`, `docker_down`, `freeze_timeout`, `store_path_missing` (a path in the running system is absent from the share, which means the host GC'd it), `oom` (the kernel killed a process for memory; detail carries the process name), `tmux_down` (no tmux server for `dev`). Each kind is sent at most once per 10 minutes. See DECISIONS I-11 and I-29 |

## Hook socket

Agents (via their wrappers) POST JSON to the Unix socket
`/run/repose/hooks.sock` (HTTP over unix, group `dev`):
`{"agent":"claude","kind":"completed","summary":"...","window":"claude"}`.
guestd relays as `AgentEvent`. `agent` must be one of the five the platform
ships, `kind` one of `completed|needs_input|error`, and `summary` is truncated
to 1 KB. `window` is optional: without it guestd resolves the calling process's
`$TMUX_PANE` through `SO_PEERCRED`, and failing that uses the agent name. The
wrapper for each agent is in `guest-conventions.md`; the payload mapping per
agent is `internal/guestd/hooks` with a recorded fixture per shape in its
`testdata/`.

## Failure behaviour

- If hostd loses the connection, it retries every 2 s; after 60 s the guest is
  marked `guestd_ok=false` in samples and an alert fires; the guest keeps
  running.
- A `Freeze` without a `Thaw` within 10 s thaws itself and returns a
  `Warning{kind:"freeze_timeout"}`.
- `Switch` output is always returned, even on failure, and failure leaves
  the previous system active (switch-to-configuration semantics).
- A request this guest's protocol version does not know is answered
  `invalid_argument` with the version in the message, so a newer hostd
  degrades per request instead of failing outright.

## Dev mode and the client

`guestd --dev-socket <path>` serves the same framing on a unix socket, for
tests and for machines without vsock. `guestd call <request> [json]` is the
client side of this document: it is what the NixOS VM test drives and what an
operator uses on a guest that has lost hostd (`ops/RUNBOOK.md`, "Guest
unresponsive"). hostd's own client is `internal/vsockrpc`.
