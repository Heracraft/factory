-- The capacity waitlist (DECISIONS I-269). A user who has never had a
-- project asks for their first one while the fleet's reserved memory is
-- near the HostMemory80 line; the create is refused with `waitlisted` and
-- the user gets a row here. The api admits waiting users oldest first as
-- room appears (or an operator does, `repose-admin waitlist admit`), and
-- admission sends one email. The row is kept after admission: admitted_at
-- is what lets the user past the gate from then on.
--
-- admitted_by: auto (the api's minute tick) or the operator's audit actor.
create table waitlist (
  user_id     uuid primary key references users(id),
  joined_at   timestamptz not null default now(),
  admitted_at timestamptz,
  admitted_by text,
  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now()
);
create index waitlist_waiting on waitlist(joined_at, user_id) where admitted_at is null;
create index waitlist_admitted on waitlist(admitted_at) where admitted_at is not null;
create trigger waitlist_updated_at before update on waitlist for each row execute function set_updated_at();

-- The admission email goes through the notification outbox like every
-- other email, but it belongs to a user who has no project yet. An event
-- now names its project or, for the few platform events about an account,
-- its user; the outbox reads the user through whichever is set.
alter table events alter column project_id drop not null;
alter table events add column user_id uuid references users(id);
alter table events add constraint events_owner check (project_id is not null or user_id is not null);
create index events_user on events(user_id, ts desc) where user_id is not null;
