# HTTP API

Base: `https://api.repose.herakraft.co/v1`. JSON. Auth: `Authorization:
Bearer <Logto access token>` for user routes (resource
`https://api.repose.herakraft.co`, verified against Logto JWKS, `sub` is the
user id). Internal routes under `/internal/` are for the gateway and use a
shared mTLS client certificate. Errors: `{ "error": { "code": "...",
"message": "...", "detail": {...} } }` with codes `unauthenticated`,
`forbidden`, `not_found`, `invalid`, `conflict`, `payment_required`,
`capacity`, `rate_limited`, `billing_disabled`, `internal`. Every response
carries `X-Request-Id`.

## Users

| Method | Path | Body / result |
|---|---|---|
| GET | `/me` | `{id, handle, email, github_login, tz, created_at, billing: {status: trial\|active\|past_due\|suspended\|exempt, trial_credit_cents, has_card}, limits: {projects, xl}}` |
| PATCH | `/me` | `{tz?, notify: {email?: bool, ntfy_url?: string\|null}}` |
| DELETE | `/me` | begins cancellation (stops guests, 30-day retention) |
| POST | `/me/notify-test` | sends a test event to every configured channel → `{email: ok\|error, ntfy: ok\|error}` |
| GET | `/notify/unsubscribe?token=` | no auth; the token is a signed, non-expiring user id (13-notifications.md §5.6) from an email's unsubscribe link. Sets `notify_email = false` and returns a plain-text confirmation; an invalid or forged token gets `invalid` |

`handle` is derived from the GitHub login at first sign-in, lowercased, `[a-z0-9-]`,
unique; it is the second half of the SSH login name.

## Projects

| Method | Path | Body / result |
|---|---|---|
| GET | `/projects` | `[Project]` |
| POST | `/projects` | `{name, remote_url?, class, tz?}` → `Project` (409 if `(user, remote_url)` or `(user, name)` exists) |
| GET | `/projects/destroyed` | `[DestroyedProject]`: the user's destroyed projects that still have a restorable snapshot, newest destroy first (I-167). New in this release |
| POST | `/projects/restore` | `{slug \| project_id \| snapshot_id, name?, start?: bool=true}` → `202 {op_id, project_id, name, slug, snapshot_id, snapshot_created_at, from_project_id}`. Restores as a new project called `name` (default: the source's name). `slug` means the live project with that slug if there is one, else the user's destroyed projects with it; the newest restorable snapshot among them is used unless `snapshot_id` names one. `404 not_found` when nothing can be restored (`detail.reason: "no_snapshot"` when the project exists); `409 conflict` with `detail: {reason: "name_taken", name}` when a live project holds the name, and with `detail.reason: "destroying"` when `slug` names a live project whose destroy has not taken its final snapshot yet (retry in a few seconds; I-190). The new project gets the source's class, volume size, configuration and, when no live project has it, its `remote_url` (I-167). New in this release |
| GET | `/projects/:id` | `Project` |
| PATCH | `/projects/:id` | `{class?, hold_base_updates?, agent_default?, tz?}` (class change requires stopped; `tz` is an IANA zone name, `400 invalid` otherwise, and is what the guest's next start writes to `/etc/repose/env`: the CLI sends it when the laptop's zone differs from the project's, I-198. `tz` is new in this release) |
| DELETE | `/projects/:id` | destroy (volume deleted, last snapshot kept 30 days) → `202 {op_id, state}`; the destroy is finished only when that op is `done` (`GET /projects/:id` then answers `404`). The project's state is `destroying` from the moment the DELETE answers; the op stops the guest, snapshots the stopped volume (reason `stop`) and deletes it (I-165). A failed destroy leaves the project in `error` with `last_error` and records a `destroy_failed` event, which notifies. A DELETE while a destroy op is open answers with that op. A dead guestd does not fail it (I-156). `state` is new in the previous release; `op_id` was always there |
| POST | `/projects/:id/start` | → `{op_id, restart}`; `restart: true` when the project was in `error` or running with its guestd not answering, and the op stops and reboots it on its newest built revision (I-157). `restart` is new in this release |
| POST | `/projects/:id/stop` | `{snapshot: bool=true}` → `{op_id}` |
| GET | `/projects/:id/ops/:op_id` | `{state: pending\|running\|done\|error, error?: {code, message, detail?, fragment_line?}, log_url?}`; `message` is the sentence to show the user, `detail` the host's own wording for operators (I-159). Answers for a destroyed project's ops too |
| GET | `/projects/:id/ops/:op_id/log` | SSE stream of `BuildLog` lines (`id:` = seq, `data:` = `{seq, line}`), then a `done` event whose data is `{state}`; `?since=<seq>` or `Last-Event-ID` resumes after a line. Browsers cannot set headers on EventSource, so this route also accepts `?access_token=<jwt>`; the token is never logged and the route is the only one that accepts it. |
| POST | `/projects/:id/resize` | `{volume_bytes}` (grow only) |
| GET | `/projects/:id/route` | `{host_id, host_name?, host_state?, guest_ip, state, host_unreachable}` (used by CLI for `status` detail; `host_name` is what it shows, I-192) |

```
Project { id, name, slug, remote_url, class, state, host_id?, guest_ip?,
          agent_default, hold_base_updates, base_version, config_revision_id,
          volume_bytes, disk_used_bytes?, created_at, started_at?,
          signals?: {ssh_sessions, tmux_clients, agents: [{agent, window, state}],
                     guestd_ok},
          cost_today_cents, cost_month_cents, last_snapshot_at?,
          last_error?, host_unreachable, tz }

DestroyedProject { id, name, slug, class, remote_url?, volume_bytes,
          destroyed_at, name_free, restorable_until?,
          snapshot: {id, created_at, bytes, reason, expires_at?} }
```

`last_error` is the sentence the last failed op left (I-159), `null`
once an op succeeds; `host_unreachable` is true while the project's host
has missed heartbeats for 90 seconds; `signals.guestd_ok` is false when
the newest sample found the environment's agent not answering (I-157).
The api has returned all three since I-157/I-159; they are documented
here since I-167. `signals` is absent until the first sample.

A `DestroyedProject`'s `snapshot` is its newest restorable snapshot and
`restorable_until` that snapshot's `expires_at` (30 days after the
destroy); `name_free` says whether a restore can take the old name.

`slug` is `name` lowercased, `[a-z0-9-]`, unique per user; it is the first
half of the SSH login name and the tmux session name.

## Config

| Method | Path | Body / result |
|---|---|---|
| GET | `/projects/:id/config` | `{revision_id, fragment, menu?: MenuSelection, base_version, applied_at?}` |
| PUT | `/projects/:id/config` | `{fragment}` or `{menu: MenuSelection}` (the api renders menu → fragment) → `{revision_id, op_id}`; build starts immediately; apply happens when the build succeeds |
| GET | `/projects/:id/config/revisions` | list with `{revision_id, created_at, status: building\|applied\|failed, error?}` |
| POST | `/projects/:id/config/revisions/:rev/apply` | re-apply an older successful revision |
| GET | `/catalog` | menu catalog: `[{id, label, group, kind, description, options?: [{id, type, values, default}]}]` (packages and services the dashboard menu offers; `kind` is `service|package|agent|runtime`, `options` are enums the menu shows as selects; from `internal/menu`, DECISIONS I-44) |

A `MenuSelection` is an array whose items are either a catalog entry
`{id, options?: {name: value}}` or any nixpkgs package by attribute path
`{package: "python312Packages.black"}` (DECISIONS I-220); an item carries
one of `id` and `package`, never both, and a `package` item has no
`options`. A `package` matches
`^[A-Za-z_][A-Za-z0-9_+-]*(\.[A-Za-z_][A-Za-z0-9_+-]*)*$`, at most 200
characters, is not a catalog id (the catalog id means the catalog entry),
and appears once; anything else is `invalid` naming it. The array may be
empty (every item removed; the project stays menu-managed). GET returns the
selection as it was PUT. The api renders catalog entries in catalog order,
then the packages by name, into the generated fragment; a package nixpkgs
does not have fails the build with `eval_failed`
`nixpkgs has no package "<name>"; search https://search.nixos.org/packages`
(nix-build-contract.md "What the user reads"). The older shape, `[{id,
options?}]` only, is the same array without `package` items and stays valid.

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
| POST | `/projects/:id/snapshots/:sid/restore` | `{as_new_project?: name, start?: bool=true}` → `{op_id, project_id}`; without `as_new_project`, replaces the stopped project's volume. `:id` may be a destroyed project. `POST /projects/restore` is the same restore resolved by name |

## Events and logs

| Method | Path | Body / result |
|---|---|---|
| GET | `/projects/:id/events?since=` | `[{id, ts, kind, agent?, summary}]` |
| GET | `/projects/:id/logs?since=&kind=console\|build\|ops` | last 10k lines, JSON lines |

## Usage and billing

| Method | Path | Body / result |
|---|---|---|
| GET | `/usage?from=&to=` | per project per day: `{guest_hours: {small,large,xl}, gb_months, egress_gb, cost_cents, credit_cents}` |
| POST | `/billing/portal` | → `{url}` (Stripe customer portal) |
| POST | `/billing/setup` | no body → `{client_secret}` for a SetupIntent (card on file); `{"flow": "checkout"}` → `{url}` of a Stripe Checkout page in setup mode that collects the card and the billing address and returns to `<dashboard>/billing?card=saved\|cancelled` (DECISIONS I-182); any other `flow` → `400 invalid` |
| GET | `/billing/invoices` | from Stripe, newest first, up to 24: `[{id, number, status, currency, amount_cents, subtotal_cents, tax_cents, created_at, period_start, period_end, hosted_url, pdf_url}]` (DECISIONS I-183; `total_cents`, `created`, `hosted_invoice_url`, `pdf` are also sent until the next release) |
| POST | `/billing/webhook` | Stripe's endpoint. No bearer token: the `Stripe-Signature` header is the authentication, verified against `STRIPE_WEBHOOK_SECRET`. Handles the six events of `09-billing.md` §5.6, idempotent on `event.id` (the `stripe_events` primary key); a duplicate answers `200 {received, duplicate}`, a bad signature `400 invalid` with the event type logged and nothing else. With no `STRIPE_SECRET_KEY` the route, like the three above, answers `503 billing_disabled` (DECISIONS I-16) |

## Internal (gateway)

| Method | Path | Body / result |
|---|---|---|
| GET | `/internal/route?login=<slug>.<handle>` | `{project_id, guest_ip, state, principals}` |
| GET | `/internal/revoked?since=` | `[serial]` |
| GET | `/internal/ca` | `{user_ca_pub, host_ca_pub}` |
| POST | `/internal/sessions` | `{project_id, event: opened\|closed, cert_serial, session_id}` (gateway reports, feeds signals; `session_id` names the relay, up to 64 of `[A-Za-z0-9-]`, so two connections under one certificate are two sessions; absent from a gateway older than I-176, accepted for one release as one session per certificate) → `{open}` |
| GET | `/internal/hosts` | `[{host_id, wg_pubkey, wg_ip, guest_cidr, state}]` for the edge's WireGuard peer sync |
| POST | `/internal/gateway-certs` | `{public_key, project_id}` → `{certificate}`: 5-minute user certificate for the gateway's own key, principal = project id, key_id suffixed `:via-gateway` |
| POST | `/internal/events` | `{source_ip, agent, kind, summary}`: hook events that reached the edge over HTTP because guestd was unavailable; the api maps `source_ip` to a project and dedupes on `(project_id, agent, kind, ts to the second)` |

## Rate limits

Per user: 600 GET requests/min, 60/min for every other method, 10/min on
`POST /certs`, 5/min on `PUT /config` (I-187: a waiting CLI polls twice a
second and shares the budget with the user's dashboard). A refusal is
`429 rate_limited` with `Retry-After` in seconds; nothing ran, so the
client may send the same request again after it. Per gateway: unlimited on
internal.

## Fake

`internal/fakes/api`: an `httptest.Server` implementing every route above
with an in-memory store, deterministic ids, and a switch to make any route
return any error code. The CLI and dashboard tests run against it.
