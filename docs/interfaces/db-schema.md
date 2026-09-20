# Database schema

Postgres 16. Migrations in `internal/db/migrations/` numbered
`NNNN_name.up.sql` / `.down.sql`, applied by `api` at start (`--migrate`) and
by `repose-admin db migrate`. Every table has `created_at timestamptz not
null default now()`; mutable tables also have `updated_at` maintained by a
trigger. Ids are `uuid` (UUIDv7 generated in Go). Money is `bigint` cents.

```sql
users        (id pk, logto_sub text unique, handle text unique, email text,
              github_login text, tz text, notify_email bool, ntfy_url text,
              stripe_customer_id text unique, billing_status text,  -- trial|active|past_due|suspended|exempt (I-16)
              has_card bool, trial_credit_cents bigint, project_limit int, xl_limit int,
              suspended_at, suspended_reason text, cancelled_at, deleted_at)

hosts        (id pk, name text unique, hostname text, sku text, provider text, region text,
              mem_bytes bigint, vcpus int, pool_bytes bigint, guest_cidr cidr,
              wg_pubkey text, wg_ip inet, state text,  -- registering|ready|draining|unreachable|retired|lost
              draining bool, free_mem_bytes bigint, pool_free_bytes bigint, load1 float, running_guests int,
              last_heartbeat_at, cert_serial text, cert_expires_at,
              nixos_system text, ch_version text,
              join_token_hash text, join_token_expires_at, registered_at)   -- the token itself is never stored

projects     (id pk, user_id fk, name text, slug text, remote_url text,
              class text, state text, host_id fk null, guest_id uuid null,
              guest_ip inet null, vsock_cid int null,
              agent_default text, hold_base_updates bool, base_version text,
              config_revision_id uuid null, volume_bytes bigint,
              tz text, host_unreachable bool, last_error text,
              started_at, stopped_at, destroyed_at,
              unique (user_id, slug) where destroyed_at is null,
              unique (user_id, remote_url) where destroyed_at is null)

host_reservations (view: host_id, reserved_bytes, guests)  -- class RAM summed over projects placed on the host in a state that holds memory

config_revisions (id pk, project_id fk, fragment text, menu jsonb null,
              base_version text, status text,  -- building|built|applied|failed
              system_closure text null, closure_bytes bigint null,
              kernel_changed bool, reboot_required bool,
              error text null, fragment_line int null, built_at, applied_at)

ops          (id pk, project_id fk null, kind text, state text,  -- pending|running|done|error
              step int, command_id uuid unique, host_id fk,
              params jsonb,          -- {phases: [...], ...} fixed at enqueue (I-42)
              command_result jsonb,  -- the hostd Result for the current phase
              result jsonb, error jsonb null, revision_id uuid, snapshot_id uuid, audit_id uuid,
              reboot_required bool, sent_at, started_at, finished_at)

build_logs   (op_id fk, seq bigint, line text, primary key (op_id, seq))

secrets      (id pk, project_id fk, name text, ciphertext bytea,
              dek_wrapped bytea, kv_key_version text, unique (project_id, name))
              -- the three reserved names of I-10 hold the guest's sshd material (I-42);
              -- the platform pseudo-project 00000000-0000-7000-8000-000000000000 holds the CA keys

certificates (serial bigint pk, user_id fk, project_ids uuid[], public_key_fp text,
              key_id text, kind text,  -- user|gateway|host
              issued_at, expires_at, revoked_at)

snapshots    (id pk, project_id fk, host_id fk, blob_path text unique, bytes bigint,
              reason text,  -- scheduled|stop|manual
              taken_at, expires_at, deleted_at, restoring_op_id uuid null)  -- set while a restore reads it; expiry skips it

events       (id pk, project_id fk, ts timestamptz, ts_second bigint, kind text, agent text null,
              tmux_window text null, summary text, source text,  -- host|http|api
              skew_seconds int null, host_event_id text unique,
              delivered jsonb)  -- {email: ts|"error: ..."|"failed: ...", ntfy: ...}
              -- unique (project_id, agent, kind, ts_second) for kinds completed|needs_input|error

events_outbox (event_id fk, channel text, attempts int, next_at, last_error text,
              primary key (event_id, channel))

meter_samples (ts timestamptz, project_id, host_id, state text, class text,
              cpu_ns bigint, mem_rss bigint, net_tx bigint, net_rx bigint,
              disk_alloc bigint, disk_used bigint, ssh_sessions int,
              tmux_clients int, agents jsonb, docker_containers int, guestd_ok bool,
              primary key (project_id, ts))  -- partitioned by month, 90-day retention

proc_samples (ts, project_id, comm text, cpu_ns bigint, rss bigint,
              primary key (project_id, ts, comm))  -- partitioned by month, 30-day retention

usage_hours  (project_id fk, hour timestamptz, class text, running_seconds int,
              gb_alloc bigint, egress_bytes bigint, cost_cents bigint,
              guest_cents bigint, storage_cents bigint, egress_cents bigint, storage_remainder bigint,
              stripe_usage_record_id text null, gap bool, primary key (project_id, hour))

invoices     (id pk, user_id fk, stripe_invoice_id text unique, period_start,
              period_end, total_cents, status text)

audit_log    (id pk, ts, actor text, action text, target text, detail jsonb)
              -- every Exec, every admin action, every cert issue and revoke

base_versions (version text pk, nix_rev text, changelog text, released_at,
              security bool)

host_sessions (host_id pk, replica_id text, since)          -- which api replica holds the host's stream
gateway_sessions (project_id fk, cert_serial bigint, opened_at, primary key (project_id, cert_serial))
settings     (key text pk, value text)                       -- edge_wg_pubkey, edge_wg_endpoint (repose-admin edge init)
schema_migrations (version int pk, name text, applied_at)
```

Indexes: `projects(user_id)`, `projects(host_id) where state in ('running',
'starting')`, `events(project_id, ts desc)`, `snapshots(project_id, taken_at
desc)`, `certificates(user_id) where revoked_at is null`, `usage_hours(hour)`,
`ops(state) where state in ('pending','running')`, `events_outbox(next_at)`.

Migrations `0001_init` and `0002_outbox_sessions_settings` create all of
this; `repose-admin db migrate --down 1` reverts one. Partitions of the
sample tables are created for the current and next month at start and by
the daily job, which also drops partitions past retention.

Rules:

- No `delete` of `projects` rows; `destroyed_at` is set and the row stays for
  usage history. `users.deleted_at` likewise.
- `meter_samples` and `proc_samples` are append-only and never joined to
  from request paths; the hourly rollup reads them once. Both are partitioned
  by month: the api creates the current and next month's partitions at start
  and in its daily job, which also drops the ones past retention and logs
  `partition_drop_fail` with `repose_api_partition_drop_fail_total` when it
  cannot. `ops/sql/partitions.sql` is the same maintenance as SQL functions,
  for an operator with psql and for the dashboard tests; either way a
  partition is dropped only once its whole month is past the retention period,
  so the effective retention is 90 or 30 days plus up to a month.
- Secrets values never appear in `audit_log.detail` or anywhere but
  `secrets.ciphertext`.
