# vsock: hostd ⇄ guestd

Inside a guest, `guestd` listens on vsock port 5000. hostd connects once per
guest (CID assigned at create) and keeps the connection; guestd accepts one
connection at a time and drops the older one. Framing: length-prefixed
protobuf (`proto/factory/guestd/v1/guestd.proto`), request/response with
`request_id`, plus unsolicited `Notify` messages from guestd.

No network listener exists in guestd. A fake for hosts without vsock (dev
machines) uses a Unix socket at `/run/factory/guestd.sock` with the same
framing; hostd picks by flag.

## Requests (hostd → guestd)

| Request | Fields | Response | Notes |
|---|---|---|---|
| `Ping` | | version, uptime_s, boot_id | also the readiness check after boot |
| `Freeze` | | | `fsfreeze -f /`; guestd sets a 10 s watchdog that thaws if no `Thaw` arrives |
| `Thaw` | | | |
| `Switch` | system_closure | rebooted (bool), output (capped 32 KB) | runs `<closure>/bin/switch-to-configuration switch`; if the closure's kernel or initrd differ from the running one, responds `needs_reboot=true` and does nothing unless `force_reboot` |
| `GrowFs` | | new_bytes | `resize2fs` after the host grew the volume |
| `WriteSecrets` | list {name, bytes} | | writes `/run/factory/secrets/<name>` 0400 dev on tmpfs, rewrites `/run/factory/secrets.env` |
| `SetPrincipals` | list | | writes `/etc/ssh/principals/dev`, reloads sshd |
| `SetupProject` | project_slug, remote_url, tz, lang | | creates tmux session named slug, `/home/dev/<slug>`, git init if empty, writes `/home/dev/.factory/project.json` |
| `Sample` | | GuestSignals + repeated ProcSample (shapes in grpc-hostd.md) | hostd calls every 60 s; guestd reads /proc and `tmux list-clients`, `tmux list-windows` |
| `Exec` | argv, timeout_s, as_user | exit_code, stdout, stderr | operator only; hostd audits every call |
| `Shutdown` | timeout_s | | `systemctl poweroff` after flushing |

## Notifications (guestd → hostd)

| Notify | Fields | Origin |
|---|---|---|
| `Ready` | boot_id | after network up and sshd listening |
| `AgentEvent` | agent, tmux_window, kind (completed\|needs_input\|error), summary | agent hooks via the unix socket `/run/factory/hooks.sock` |
| `AgentState` | agent, tmux_window, state | on change, debounced 5 s |
| `Warning` | kind, detail | kinds: `disk_high` (over 90 percent), `inotify_exhausted`, `docker_down`, `freeze_timeout`, `store_path_missing` (a path in the running system is absent from the share, which means the host GC'd it) |

## Hook socket

Agents (via their wrappers) POST JSON to the Unix socket
`/run/factory/hooks.sock` (HTTP over unix, group `dev`):
`{"agent":"claude","kind":"completed","summary":"..."}`. guestd relays as
`AgentEvent`. The wrapper for each agent is in `guest-conventions.md`.

## Failure behaviour

- If hostd loses the connection, it retries every 2 s; after 60 s the guest is
  marked `guestd_ok=false` in samples and an alert fires; the guest keeps
  running.
- A `Freeze` without a `Thaw` within 10 s thaws itself and returns a
  `Warning{kind:"freeze_timeout"}`.
- `Switch` output is always returned, even on failure, and failure leaves
  the previous system active (switch-to-configuration semantics).
