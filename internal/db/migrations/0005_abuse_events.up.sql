-- Automatic abuse stops (DECISIONS I-239). A row is one stop the api made
-- because a guest's process samples named a cryptocurrency miner. `hold`
-- is set on the stop that makes three uncleared ones within 24 hours; a
-- project with an uncleared held row cannot be started until an operator
-- runs `repose-admin abuse clear <project>`, which sets cleared_at and
-- cleared_by on all of its rows. The user is never suspended from here.
create table abuse_events (
  id          uuid primary key,
  project_id  uuid not null references projects(id),
  user_id     uuid not null references users(id),
  ts          timestamptz not null default now(),
  kind        text not null,                -- miner
  detail      jsonb not null default '{}',  -- {"process": "<name>"}: a process name, never arguments
  op_id       uuid,                         -- the stop op
  hold        boolean not null default false,
  cleared_at  timestamptz,
  cleared_by  text,
  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now()
);
create index abuse_events_project on abuse_events(project_id, ts desc);
create trigger abuse_events_updated_at before update on abuse_events for each row execute function set_updated_at();
