-- Workstream 09 (billing). The credit ledger of 09-billing.md §5.3, the
-- Stripe webhook dedupe table of §5.6, and the columns the billing period
-- needs on usage_hours and users.
--
-- "Month" is the user's Stripe billing period anchored at signup (§5.1),
-- not the calendar month, so the cap, the egress allowance and the storage
-- remainder are all carried per period and the period each hour belongs to
-- is written on the row.

create table credit_ledger (
  id         uuid primary key,
  user_id    uuid not null references users(id),
  cents      bigint not null,
  reason     text not null,          -- trial|usage|goodwill|refund|adjustment
  ref        text,                   -- '<project_id>:<hour>' for a usage debit
  created_at timestamptz not null default now()
);
create index credit_ledger_user on credit_ledger(user_id, created_at);
-- One usage debit per usage_hours row; a re-run that computes a different
-- cost adds an 'adjustment' row, so the ledger stays append-only.
create unique index credit_ledger_usage_ref on credit_ledger(ref) where reason = 'usage';

-- Existing accounts keep the balance they have: one 'trial' row each, so
-- sum(credit_ledger.cents) equals users.trial_credit_cents from the first
-- hour after this migration. Backfilled before the trigger below exists.
insert into credit_ledger (id, user_id, cents, reason)
  select gen_random_uuid(), id, trial_credit_cents, 'trial' from users where trial_credit_cents <> 0;

-- users.trial_credit_cents is a projection of the ledger, maintained inside
-- the same transaction as the ledger row (and therefore under the same row
-- lock), so the drift §5.3 rejected a cached column for cannot happen. The
-- balance of record is still sum(credit_ledger.cents).
create function credit_ledger_apply() returns trigger as $$
begin
  update users set trial_credit_cents = trial_credit_cents + new.cents where id = new.user_id;
  return new;
end;
$$ language plpgsql;
create trigger credit_ledger_apply after insert on credit_ledger for each row execute function credit_ledger_apply();

create table stripe_events (
  id           text primary key,     -- Stripe's event.id; the dedupe key (§5.6)
  type         text not null,
  received_at  timestamptz not null default now(),
  processed_at timestamptz,
  error        text
);
create index stripe_events_received on stripe_events(received_at desc);

alter table usage_hours
  add column period_start  timestamptz,
  add column period_end    timestamptz,
  add column credit_cents  bigint not null default 0,
  add column price_version text not null default 'v1';
-- The hourly push walks the rows that have no Stripe record yet (§6).
create index usage_hours_push_pending on usage_hours(hour) where stripe_usage_record_id is null;
create index usage_hours_period on usage_hours(project_id, period_start);

alter table users
  add column stripe_subscription_id text unique,
  add column billing_anchor timestamptz,
  add column past_due_since timestamptz;
-- An account that existed before billing anchors on its signup (§5.1).
update users set billing_anchor = created_at where billing_anchor is null;
