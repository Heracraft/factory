-- Schema reference: docs/interfaces/db-schema.md. Ids are UUIDv7 generated
-- in Go; money is bigint cents; every table has created_at and mutable
-- tables carry updated_at maintained by set_updated_at().

create or replace function set_updated_at() returns trigger language plpgsql as $$
begin
  new.updated_at = now();
  return new;
end $$;

create table users (
  id                 uuid primary key,
  logto_sub          text unique,
  handle             text unique not null,
  email              text,
  github_login       text,
  tz                 text,
  notify_email       boolean not null default true,
  ntfy_url           text,
  stripe_customer_id text unique,
  billing_status     text not null default 'trial'
                     check (billing_status in ('trial','active','past_due','suspended','exempt')),
  has_card           boolean not null default false,
  trial_credit_cents bigint not null default 1000,
  project_limit      integer not null default 3,
  xl_limit           integer not null default 1,
  suspended_at       timestamptz,
  suspended_reason   text,
  cancelled_at       timestamptz,
  deleted_at         timestamptz,
  created_at         timestamptz not null default now(),
  updated_at         timestamptz not null default now()
);
create trigger users_updated_at before update on users for each row execute function set_updated_at();

create sequence host_seq start 1;

create table hosts (
  id                    uuid primary key,
  name                  text unique not null,
  hostname              text,
  sku                   text,
  provider              text,
  region                text,
  mem_bytes             bigint not null default 0,
  vcpus                 integer not null default 0,
  pool_bytes            bigint not null default 0,
  guest_cidr            cidr,
  wg_pubkey             text,
  wg_ip                 inet,
  state                 text not null default 'registering'
                        check (state in ('registering','ready','draining','unreachable','retired','lost')),
  draining              boolean not null default false,
  free_mem_bytes        bigint not null default 0,
  pool_free_bytes       bigint not null default 0,
  load1                 double precision not null default 0,
  running_guests        integer not null default 0,
  last_heartbeat_at     timestamptz,
  cert_serial           text,
  cert_expires_at       timestamptz,
  nixos_system          text,
  ch_version            text,
  join_token_hash       text,
  join_token_expires_at timestamptz,
  registered_at         timestamptz,
  created_at            timestamptz not null default now(),
  updated_at            timestamptz not null default now()
);
create trigger hosts_updated_at before update on hosts for each row execute function set_updated_at();

create table base_versions (
  version     text primary key,
  nix_rev     text not null,
  changelog   text not null default '',
  released_at timestamptz not null default now(),
  security    boolean not null default false,
  created_at  timestamptz not null default now()
);

create table projects (
  id                 uuid primary key,
  user_id            uuid not null references users(id),
  name               text not null,
  slug               text not null,
  remote_url         text,
  class              text not null check (class in ('small','large','xl')),
  state              text not null check (state in ('creating','building','starting','running','stopping','stopped','restoring','destroying','destroyed','error')),
  host_id            uuid references hosts(id),
  guest_id           uuid,
  guest_ip           inet,
  vsock_cid          integer,
  agent_default      text not null default 'claude',
  hold_base_updates  boolean not null default false,
  base_version       text,
  config_revision_id uuid,
  volume_bytes       bigint not null,
  tz                 text,
  host_unreachable   boolean not null default false,
  last_error         text,
  started_at         timestamptz,
  stopped_at         timestamptz,
  destroyed_at       timestamptz,
  created_at         timestamptz not null default now(),
  updated_at         timestamptz not null default now()
);
create trigger projects_updated_at before update on projects for each row execute function set_updated_at();
create unique index projects_user_slug on projects(user_id, slug) where destroyed_at is null;
create unique index projects_user_remote on projects(user_id, remote_url) where destroyed_at is null and remote_url is not null;
create index projects_user on projects(user_id);
create index projects_host_live on projects(host_id) where state in ('running','starting');

-- Memory reserved per host: every project placed on it whose guest holds
-- or is about to hold memory. Stopped and destroyed projects hold none.
create view host_reservations as
  select host_id,
         coalesce(sum(case class when 'small' then 4::bigint<<30 when 'large' then 8::bigint<<30 else 16::bigint<<30 end), 0)::bigint as reserved_bytes,
         count(*)::integer as guests
    from projects
   where host_id is not null
     and state in ('creating','building','starting','running','stopping','restoring')
   group by host_id;

create table config_revisions (
  id              uuid primary key,
  project_id      uuid not null references projects(id),
  fragment        text not null,
  menu            jsonb,
  base_version    text,
  status          text not null check (status in ('building','built','applied','failed')),
  system_closure  text,
  closure_bytes   bigint,
  kernel_changed  boolean not null default false,
  reboot_required boolean not null default false,
  error           text,
  fragment_line   integer,
  built_at        timestamptz,
  applied_at      timestamptz,
  created_at      timestamptz not null default now(),
  updated_at      timestamptz not null default now()
);
create trigger config_revisions_updated_at before update on config_revisions for each row execute function set_updated_at();
create index config_revisions_project on config_revisions(project_id, created_at desc);

create table ops (
  id              uuid primary key,
  project_id      uuid references projects(id),
  kind            text not null,
  state           text not null check (state in ('pending','running','done','error')),
  step            integer not null default 0,
  command_id      uuid,
  host_id         uuid references hosts(id),
  params          jsonb not null default '{}'::jsonb,
  command_result  jsonb,
  result          jsonb,
  error           jsonb,
  revision_id     uuid,
  snapshot_id     uuid,
  audit_id        uuid,
  reboot_required boolean not null default false,
  sent_at         timestamptz,
  started_at      timestamptz,
  finished_at     timestamptz,
  created_at      timestamptz not null default now(),
  updated_at      timestamptz not null default now()
);
create trigger ops_updated_at before update on ops for each row execute function set_updated_at();
create index ops_project on ops(project_id, created_at desc);
create index ops_open on ops(state) where state in ('pending','running');
create unique index ops_command on ops(command_id) where command_id is not null;

create table build_logs (
  op_id uuid not null references ops(id),
  seq   bigint not null,
  line  text not null,
  primary key (op_id, seq)
);

create table secrets (
  id             uuid primary key,
  project_id     uuid not null references projects(id),
  name           text not null,
  ciphertext     bytea not null,
  dek_wrapped    bytea not null,
  kv_key_version text not null,
  created_at     timestamptz not null default now(),
  updated_at     timestamptz not null default now(),
  unique (project_id, name)
);
create trigger secrets_updated_at before update on secrets for each row execute function set_updated_at();

create sequence certificates_serial start 1;

create table certificates (
  serial        bigint primary key default nextval('certificates_serial'),
  user_id       uuid not null references users(id),
  project_ids   uuid[] not null default '{}',
  public_key_fp text not null,
  key_id        text not null,
  kind          text not null default 'user' check (kind in ('user','gateway','host')),
  issued_at     timestamptz not null default now(),
  expires_at    timestamptz not null,
  revoked_at    timestamptz,
  created_at    timestamptz not null default now()
);
create index certificates_user_live on certificates(user_id) where revoked_at is null;
create index certificates_revoked on certificates(revoked_at) where revoked_at is not null;

create table snapshots (
  id               uuid primary key,
  project_id       uuid not null references projects(id),
  host_id          uuid references hosts(id),
  blob_path        text not null unique,
  bytes            bigint not null default 0,
  reason           text not null check (reason in ('scheduled','stop','manual')),
  taken_at         timestamptz not null default now(),
  expires_at       timestamptz,
  deleted_at       timestamptz,
  restoring_op_id  uuid,
  created_at       timestamptz not null default now()
);
create index snapshots_project on snapshots(project_id, taken_at desc);

create table events (
  id            uuid primary key,
  project_id    uuid not null references projects(id),
  ts            timestamptz not null,
  ts_second     bigint not null,   -- unix seconds of ts, the dedupe key
  kind          text not null,
  agent         text,
  tmux_window   text,
  summary       text not null default '',
  source        text not null default 'host',
  skew_seconds  integer,
  host_event_id text unique,
  delivered     jsonb not null default '{}'::jsonb,
  created_at    timestamptz not null default now()
);
create index events_project_ts on events(project_id, ts desc);
create unique index events_dedupe on events(project_id, coalesce(agent,''), kind, ts_second);

create table meter_samples (
  ts                timestamptz not null,
  project_id        uuid not null,
  host_id           uuid,
  state             text not null,
  class             text not null,
  cpu_ns            bigint not null default 0,
  mem_rss           bigint not null default 0,
  net_tx            bigint not null default 0,
  net_rx            bigint not null default 0,
  disk_alloc        bigint not null default 0,
  disk_used         bigint not null default 0,
  ssh_sessions      integer not null default 0,
  tmux_clients      integer not null default 0,
  agents            jsonb not null default '[]'::jsonb,
  docker_containers integer not null default 0,
  guestd_ok         boolean not null default true,
  primary key (project_id, ts)
) partition by range (ts);

create table proc_samples (
  ts         timestamptz not null,
  project_id uuid not null,
  comm       text not null,
  cpu_ns     bigint not null default 0,
  rss        bigint not null default 0,
  primary key (project_id, ts, comm)
) partition by range (ts);

create table usage_hours (
  project_id              uuid not null references projects(id),
  hour                    timestamptz not null,
  class                   text not null,
  running_seconds         integer not null default 0,
  gb_alloc                bigint not null default 0,
  egress_bytes            bigint not null default 0,
  cost_cents              bigint not null default 0,
  stripe_usage_record_id  text,
  gap                     boolean not null default false,
  created_at              timestamptz not null default now(),
  updated_at              timestamptz not null default now(),
  primary key (project_id, hour)
);
create trigger usage_hours_updated_at before update on usage_hours for each row execute function set_updated_at();
create index usage_hours_hour on usage_hours(hour);

create table invoices (
  id                uuid primary key,
  user_id           uuid not null references users(id),
  stripe_invoice_id text unique,
  period_start      timestamptz,
  period_end        timestamptz,
  total_cents       bigint not null default 0,
  status            text not null,
  created_at        timestamptz not null default now()
);

create table audit_log (
  id         uuid primary key,
  ts         timestamptz not null default now(),
  actor      text not null,
  action     text not null,
  target     text not null default '',
  detail     jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);
create index audit_log_ts on audit_log(ts desc);

-- The platform pseudo-user and pseudo-project own the CA material in the
-- secrets table (05-control-plane-api.md §5.5). Nothing schedules or
-- lists them: the state is destroyed from birth.
insert into users (id, handle, email, billing_status, project_limit, xl_limit, trial_credit_cents)
  values ('00000000-0000-7000-8000-000000000000', 'repose-platform', null, 'exempt', 0, 0, 0);
insert into projects (id, user_id, name, slug, class, state, volume_bytes, destroyed_at)
  values ('00000000-0000-7000-8000-000000000000', '00000000-0000-7000-8000-000000000000', 'platform', 'platform', 'small', 'destroyed', 0, now());
