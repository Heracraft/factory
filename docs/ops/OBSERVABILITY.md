# Observability, for operators

Where every signal lives, how to look at one project, and what must never
be in a log. The design rules and the full metric list are in
`../workstreams/10-observability.md`.

## Where things are

```
hosts (NixOS)          journald ─▶ Fluent Bit ─▶ Loki  (labels host, component, guest_id for console)
                       hostd :9101 /metrics, node_exporter :9100  ◀─ Prometheus scrape over WireGuard
edge (NixOS)           gateway :9102 /metrics; journald ─▶ Fluent Bit ─▶ Loki
Coolify VM (Ubuntu)    api, web containers: stdout ─▶ Coolify log drain ─▶ Loki; api :9103 /metrics over WireGuard
Postgres               meter_samples, proc_samples, usage_hours, events, audit_log (Grafana Postgres datasource, read-only role)
personal server        Loki, Prometheus, Grafana, Alertmanager (ntfy to the owner)
```

Nothing observability-related runs on hosts beyond Fluent Bit and the two
exporters, and nothing is reachable from the internet: every scrape and
ship goes over the edge's WireGuard.

## Looking at one project

1. `factory-admin projects show <id-or-slug>`: state, host, class, last
   snapshot, current signals, cost today. This is the first stop.
2. Grafana "Per-guest resources", variable `project_id`: CPU, memory, net,
   disk, and the signals timeline (sessions, tmux clients, agent state)
   from `meter_samples`.
3. Logs. Loki queries:
   - hostd events for the guest: `{component="hostd"} | json | guest_id="<guest id>"`
   - the guest's console: `{component="console", guest_id="<guest id>"}`
   - api requests: `{component="api"} | json | project_id="<id>"`
   - gateway sessions: `{component="gateway"} | json | event=~"session_.*" | project_id="<id>"`
4. Builds: `factory-admin ops list --project <id>` then `ops log <op id>`
   for the full Nix output (stored in `build_logs`, not Loki).
5. Events the user saw: `select ts, kind, agent, summary, delivered from
   events where project_id = ... order by ts desc`.

## Looking at one host

Grafana "Host capacity", variable `host_id`. Then on the host itself:
`systemctl list-units 'guest@*'`, `lvs vg-guests`, `nft list counters`,
`journalctl -u hostd -f`.

## Looking for abuse

Grafana "Abuse": fleet-wide top `comm` by CPU over 24 hours, top projects
by egress, and guests at 100 percent CPU with zero sessions for over 24
hours. A miner is a `comm` you do not recognise at the top of the first
panel. Confirm with `factory-admin exec <id> -- ps -o comm,pcpu --sort
-pcpu | head` (audited), then the runbook's "Suspend a user".

## Log field rules

Every line: `ts`, `level`, `component`, `event`, `msg`. Context when it
exists: `request_id`, `command_id`, `op_id`, `project_id`, `guest_id`,
`host_id`, `user_id`, and bounded enums (`state`, `reason`, `kind`,
`class`, `result`), counts, byte sizes, durations in milliseconds.

**Never in a log field, a metric label, a trace attribute, or a build log
line stored by us:**

- prompts, agent output, terminal contents, anything typed in a guest
- process command-line arguments or environment variables (process
  *names* are fine, that is the documented sample)
- file paths inside a guest (`/home/dev/todo-app/src/secret.ts` is a
  tenant's business)
- secret names together with values; values ever
- tokens, JWTs, refresh tokens, certificate bodies, private keys, join
  tokens
- user email, GitHub login, handle (use `user_id`)
- git remote URLs (they can carry embedded tokens); log `project_id`
- user IP addresses and user agents (the gateway may keep a per-IP
  counter in memory for rate limiting; it does not log the address)
- Stripe card details of any kind, including last four

`internal/obs` redacts fields named `token`, `secret`, `password`,
`authorization`, `cert`, `key` as a floor; the reviewer is the real check.
The reason is in the privacy policy: it promises the process-sample
boundary in plain words, and a log line that crosses it is a broken
promise that outlives the incident.

## Retention

| Store | Keeps |
|---|---|
| Loki console logs | 30 days |
| Loki component logs | 90 days |
| Prometheus | 90 days |
| `meter_samples` | 90 days (monthly partitions) |
| `proc_samples` | 30 days |
| `usage_hours`, `events`, `audit_log`, `build_logs` | indefinite (`build_logs` trimmed to the last 20 ops per project) |

## Alerts

Routed by Alertmanager to the owner's ntfy topic. Each alert name is a
heading in `RUNBOOK.md`. Silence with `amtool silence add alertname=...
--duration 2h --comment "..."` and say why in the comment; a silence with
no comment is deleted by a nightly job.
