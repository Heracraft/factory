# Database schema

Postgres 16. Migrations in `internal/db/migrations/` numbered
`NNNN_name.up.sql` / `.down.sql`, applied by `api` at start (`--migrate`) and
by `factory-admin db migrate`. Every table has `created_at timestamptz not
null default now()`; mutable tables also have `updated_at` maintained by a
trigger. Ids are `uuid` (UUIDv7 generated in Go). Money is `bigint` cents.

```sql
users        (id pk, logto_sub text unique, handle text unique, email text,
              github_login text, tz text, notify_email bool, ntfy_url text,
              stripe_customer_id text unique, billing_status text,
              trial_credit_cents bigint, project_limit int, xl_limit int,
              suspended_at, cancelled_at, deleted_at)

hosts        (id pk, hostname text, sku text, provider text, region text,
              mem_bytes bigint, vcpus int, pool_bytes bigint, guest_cidr cidr,
              wg_pubkey text, wg_ip inet, state text,  -- registering|ready|draining|unreachable|retired
              last_heartbeat_at, cert_serial text, cert_expires_at,
              nixos_system text, ch_version text)

projects     (id pk, user_id fk, name text, slug text, remote_url text,
              class text, state text, host_id fk null, guest_id uuid null,
              guest_ip inet null, vsock_cid int null,
              agent_default text, hold_base_updates bool, base_version text,
              config_revision_id uuid null, volume_bytes bigint,
              tz text, started_at, stopped_at, destroyed_at,
              unique (user_id, slug), unique (user_id, remote_url))

config_revisions (id pk, project_id fk, fragment text, menu jsonb null,
              base_version text, status text,  -- building|built|applied|failed
              system_closure text null, closure_bytes bigint null,
              error text null, fragment_line int null, built_at, applied_at)

ops          (id pk, project_id fk, kind text, state text, command_id uuid,
              host_id fk, error jsonb null, started_at, finished_at)

build_logs   (op_id fk, seq int, line text, primary key (op_id, seq))

secrets      (id pk, project_id fk, name text, ciphertext bytea,
              dek_wrapped bytea, kv_key_version text, unique (project_id, name))

certificates (serial bigint pk, user_id fk, project_ids uuid[], public_key_fp text,
              issued_at, expires_at, revoked_at)

snapshots    (id pk, project_id fk, host_id fk, blob_path text, bytes bigint,
              reason text, taken_at, expires_at, deleted_at)

events       (id pk, project_id fk, ts timestamptz, kind text, agent text null,
              summary text, delivered jsonb)  -- {email: ts|error, ntfy: ts|error}

meter_samples (ts timestamptz, project_id fk, host_id fk, state text, class text,
              cpu_ns bigint, mem_rss bigint, net_tx bigint, net_rx bigint,
              disk_alloc bigint, disk_used bigint, ssh_sessions int,
              tmux_clients int, agents jsonb, docker_containers int,
              primary key (project_id, ts))  -- partitioned by month, 90-day retention

proc_samples (ts, project_id fk, comm text, cpu_ns bigint, rss bigint,
              primary key (project_id, ts, comm))  -- partitioned by month, 30-day retention

usage_hours  (project_id fk, hour timestamptz, class text, running_seconds int,
              gb_alloc bigint, egress_bytes bigint, cost_cents bigint,
              stripe_usage_record_id text null, primary key (project_id, hour))

invoices     (id pk, user_id fk, stripe_invoice_id text unique, period_start,
              period_end, total_cents, status text)

audit_log    (id pk, ts, actor text, action text, target text, detail jsonb)
              -- every Exec, every admin action, every cert issue and revoke

base_versions (version text pk, nix_rev text, changelog text, released_at,
              security bool)
```

Indexes: `projects(user_id)`, `projects(host_id) where state in ('running',
'starting')`, `events(project_id, ts desc)`, `snapshots(project_id, taken_at
desc)`, `certificates(user_id) where revoked_at is null`, `usage_hours(hour)`.

Rules:

- No `delete` of `projects` rows; `destroyed_at` is set and the row stays for
  usage history. `users.deleted_at` likewise.
- `meter_samples` and `proc_samples` are append-only and never joined to
  from request paths; the hourly rollup reads them once.
- Secrets values never appear in `audit_log.detail` or anywhere but
  `secrets.ciphertext`.
