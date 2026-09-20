-- The notification outbox (13-notifications.md §5.5), the replica map for
-- host streams (05-control-plane-api.md §5.1) and platform-level settings
-- such as the edge WireGuard hub written by `repose-admin edge init`.

create table events_outbox (
  event_id   uuid not null references events(id),
  channel    text not null,
  attempts   integer not null default 0,
  next_at    timestamptz not null default now(),
  last_error text,
  created_at timestamptz not null default now(),
  primary key (event_id, channel)
);
create index events_outbox_due on events_outbox(next_at);

create table host_sessions (
  host_id    uuid primary key references hosts(id),
  replica_id text not null,
  since      timestamptz not null default now(),
  created_at timestamptz not null default now()
);

create table settings (
  key        text primary key,
  value      text not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create trigger settings_updated_at before update on settings for each row execute function set_updated_at();
