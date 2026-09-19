# HTTP API

Base: `https://api.repose.herakraft.co/v1`. JSON. Auth: `Authorization:
Bearer <Logto access token>` for user routes (resource
`https://api.repose.herakraft.co`, verified against Logto JWKS, `sub` is the
user id). Internal routes under `/internal/` are for the gateway and use a
shared mTLS client certificate. Errors: `{ "error": { "code": "...",
"message": "...", "detail": {...} } }` with codes `unauthenticated`,
`forbidden`, `not_found`, `invalid`, `conflict`, `payment_required`,
`capacity`, `rate_limited`, `internal`. Every response carries
`X-Request-Id`.

## Users

| Method | Path | Body / result |
|---|---|---|
| GET | `/me` | `{id, handle, email, github_login, tz, created_at, billing: {status: trial\|active\|past_due\|suspended\|exempt, trial_credit_cents, has_card}, limits: {projects, xl}}` |
| PATCH | `/me` | `{tz?, notify: {email?: bool, ntfy_url?: string\|null}}` |
| DELETE | `/me` | begins cancellation (stops guests, 30-day retention) |
| POST | `/me/notify-test` | sends a test event to every configured channel → `{email: ok\|error, ntfy: ok\|error}` |

`handle` is derived from the GitHub login at first sign-in, lowercased, `[a-z0-9-]`,
unique; it is the second half of the SSH login name.

## Projects

| Method | Path | Body / result |
|---|---|---|
| GET | `/projects` | `[Project]` |
| POST | `/projects` | `{name, remote_url?, class, tz?}` → `Project` (409 if `(user, remote_url)` or `(user, name)` exists) |
| GET | `/projects/:id` | `Project` |
| PATCH | `/projects/:id` | `{class?, hold_base_updates?, agent_default?}` (class change requires stopped) |
| DELETE | `/projects/:id` | destroy (volume deleted, last snapshot kept 30 days) |
| POST | `/projects/:id/start` | → `{op_id}` |
| POST | `/projects/:id/stop` | `{snapshot: bool=true}` → `{op_id}` |
| GET | `/projects/:id/ops/:op_id` | `{state: pending\|running\|done\|error, error?, log_url?}` |
| GET | `/projects/:id/ops/:op_id/log` | SSE stream of `BuildLog` lines, then `done` event. Browsers cannot set headers on EventSource, so this route also accepts `?access_token=<jwt>`; the token is never logged and the route is the only one that accepts it. |
| POST | `/projects/:id/resize` | `{volume_bytes}` (grow only) |
| GET | `/projects/:id/route` | `{host_id, guest_ip, state}` (used by CLI for `status` detail) |

```
Project { id, name, slug, remote_url, class, state, host_id?, guest_ip?,
          agent_default, hold_base_updates, base_version, config_revision_id,
          volume_bytes, disk_used_bytes?, created_at, started_at?,
          signals?: {ssh_sessions, tmux_clients, agents: [{agent, window, state}]},
          cost_today_cents, cost_month_cents, last_snapshot_at? }
```

`slug` is `name` lowercased, `[a-z0-9-]`, unique per user; it is the first
half of the SSH login name and the tmux session name.

## Config

| Method | Path | Body / result |
|---|---|---|
| GET | `/projects/:id/config` | `{revision_id, fragment, menu?: MenuSelection, base_version, applied_at?}` |
| PUT | `/projects/:id/config` | `{fragment}` or `{menu: MenuSelection}` (the api renders menu → fragment) → `{revision_id, op_id}`; build starts immediately; apply happens when the build succeeds |
| GET | `/projects/:id/config/revisions` | list with `{revision_id, created_at, status: building\|applied\|failed, error?}` |
| POST | `/projects/:id/config/revisions/:rev/apply` | re-apply an older successful revision |
| GET | `/catalog` | menu catalog: `[{id, label, group, description}]` (packages and services the dashboard menu offers) |

## Certificates

| Method | Path | Body / result |
|---|---|---|
| POST | `/certs` | `{public_key (OpenSSH format), project_ids: [..]}` → `{certificate (OpenSSH cert), expires_at, gateway: {host, port, host_ca_pub}}`; one cert may carry several principals |
| POST | `/certs/revoke` | `{serial}` or `{all: true}` |

## Secrets

| Method | Path | Body / result |
|---|---|---|
| GET | `/projects/:id/secrets` | `[{name, created_at, updated_at}]` (never values) |
| PUT | `/projects/:id/secrets/:name` | `{value}` (base64, max 64 KB) → pushed to a running guest via `UpdateSecrets` |
| DELETE | `/projects/:id/secrets/:name` | |

Names: `[A-Z][A-Z0-9_]{0,63}`. The names `ssh_host_ed25519_key`,
`ssh_host_ed25519_key-cert.pub` and `user_ca.pub` are reserved for the guest's
sshd material (delivered by hostd into the same tmpfs from the explicit
`CreateGuest` fields, see I-3 and I-10) and are rejected with `invalid`.

## Snapshots

| Method | Path | Body / result |
|---|---|---|
| GET | `/projects/:id/snapshots` | `[{id, created_at, bytes, reason}]` |
| POST | `/projects/:id/snapshots` | manual snapshot → `{op_id}` |
| POST | `/projects/:id/snapshots/:sid/restore` | `{as_new_project?: name}` → `{op_id}`; without `as_new_project`, replaces the stopped project's volume |

## Events and logs

| Method | Path | Body / result |
|---|---|---|
| GET | `/projects/:id/events?since=` | `[{id, ts, kind, agent?, summary}]` |
| GET | `/projects/:id/logs?since=&kind=console\|build\|ops` | last 10k lines, JSON lines |

## Usage and billing

| Method | Path | Body / result |
|---|---|---|
| GET | `/usage?from=&to=` | per project per day: `{guest_hours: {small,large,xl}, gb_months, egress_gb, cost_cents}` |
| POST | `/billing/portal` | → `{url}` (Stripe customer portal) |
| POST | `/billing/setup` | → `{client_secret}` for a SetupIntent (card on file) |
| GET | `/billing/invoices` | from Stripe, cached 5 min |

## Internal (gateway)

| Method | Path | Body / result |
|---|---|---|
| GET | `/internal/route?login=<slug>.<handle>` | `{project_id, guest_ip, state, principals}` |
| GET | `/internal/revoked?since=` | `[serial]` |
| GET | `/internal/ca` | `{user_ca_pub, host_ca_pub}` |
| POST | `/internal/sessions` | `{project_id, opened\|closed, cert_serial}` (gateway reports, feeds signals) |
| GET | `/internal/hosts` | `[{host_id, wg_pubkey, wg_ip, guest_cidr, state}]` for the edge's WireGuard peer sync |
| POST | `/internal/gateway-certs` | `{public_key, project_id}` → `{certificate}`: 5-minute user certificate for the gateway's own key, principal = project id, key_id suffixed `:via-gateway` |
| POST | `/internal/events` | `{source_ip, agent, kind, summary}`: hook events that reached the edge over HTTP because guestd was unavailable; the api maps `source_ip` to a project and dedupes on `(project_id, agent, kind, ts to the second)` |

## Rate limits

Per user: 60 requests/min general, 10/min on `POST /certs`, 5/min on
`PUT /config`. Per gateway: unlimited on internal.

## Fake

`internal/fakes/api`: an `httptest.Server` implementing every route above
with an in-memory store, deterministic ids, and a switch to make any route
return any error code. The CLI and dashboard tests run against it.
