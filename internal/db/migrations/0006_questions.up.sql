-- Questions an agent asked with repose-ask (DECISIONS I-244, I-245). The id
-- is the one guestd chose (UUIDv7); a guest re-announces an open question
-- on every new hostd connection, and the insert ignores the repeat.
--
-- text and answer are tenant content stored the way events.summary is:
-- plain text, shown to the owner on their channels, the dashboard and the
-- CLI, and never written to a log line, a metric or a trace. They are not
-- secrets and live nowhere a secret does.
--
-- state: pending until answered, cancelled (the owner dismissed it, the
-- asker gave up, or the guest stopped), expired (its timeout passed) or
-- no_channel (the owner had no channel on when it was asked). Every close
-- the guest did not make itself is carried back to it by AnswerQuestion;
-- delivered_at is when the guest acknowledged that, or when the api
-- stopped trying (delivery says which).
create table questions (
  id                 uuid primary key,
  project_id         uuid not null references projects(id),
  guest_id           uuid not null,
  event_id           uuid not null,     -- the events row that notified the owner
  agent              text not null,     -- one of the five, or shell
  tmux_window        text,
  text               text not null,
  options            text[] not null default '{}',
  state              text not null default 'pending',
  answer             text,
  answered_via       text,              -- dashboard|cli|ntfy|email
  expires_at         timestamptz not null,
  answered_at        timestamptz,
  delivered_at       timestamptz,
  delivery           text,              -- ok|gone|given_up|guest (the guest closed it itself)
  deliver_command_id uuid,
  deliver_attempts   integer not null default 0,
  deliver_next_at    timestamptz,
  created_at         timestamptz not null default now(),
  updated_at         timestamptz not null default now(),
  constraint questions_state check (state in ('pending','answered','cancelled','expired','no_channel'))
);
create index questions_project on questions(project_id, created_at desc);
create index questions_pending on questions(expires_at) where state = 'pending';
create index questions_undelivered on questions(deliver_next_at) where state <> 'pending' and delivered_at is null;
create index questions_command on questions(deliver_command_id) where deliver_command_id is not null;
create trigger questions_updated_at before update on questions for each row execute function set_updated_at();
