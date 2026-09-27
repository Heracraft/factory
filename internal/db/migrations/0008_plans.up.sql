-- Monthly plans through Paddle (DECISIONS I-289) and seats (I-290). The
-- subscription is the record; users.billing_status is a projection of it
-- and gains 'none' for an account that has never chosen a plan. Stripe's
-- columns go: the customer id is renamed (Paddle's customer takes its
-- place), the subscription id lives in its own table, and the event dedupe
-- table is Paddle's. The credit ledger and usage_hours stay as they are:
-- the ledger is history, usage_hours is the internal record of hours, disk
-- and egress the overage line is computed from.

alter table users rename column stripe_customer_id to paddle_customer_id;
alter table users drop column stripe_subscription_id;
alter table users drop constraint users_billing_status_check;
alter table users add constraint users_billing_status_check
  check (billing_status in ('none','trial','active','past_due','suspended','exempt'));
alter table users alter column billing_status set default 'none';
alter table users alter column trial_credit_cents set default 0;

-- One row per Paddle subscription, ids are Paddle's (sub_...). A user has
-- at most one live subscription; cancelled ones stay for the invoices and
-- the count.
create table subscriptions (
  id                  text primary key,
  user_id             uuid not null references users(id),
  paddle_customer_id  text not null,
  plan                text not null check (plan in ('solo','pro')),
  status              text not null check (status in ('trialing','active','past_due','paused','canceled')),
  seats               integer not null,
  period_start        timestamptz,
  period_end          timestamptz,
  next_billed_at      timestamptz,
  trial_end           timestamptz,
  cancel_at           timestamptz,               -- a scheduled cancellation takes effect here
  scheduled_plan      text check (scheduled_plan in ('solo','pro')),  -- a downgrade waiting for period_end
  overage_charged_for timestamptz,               -- period_start of the last period whose egress line was sent
  created_at          timestamptz not null default now(),
  updated_at          timestamptz not null default now()
);
create unique index subscriptions_live_user on subscriptions(user_id)
  where status in ('trialing','active','past_due','paused');
create index subscriptions_user on subscriptions(user_id, created_at desc);
create index subscriptions_next_billed on subscriptions(next_billed_at)
  where status in ('trialing','active','past_due');
create trigger subscriptions_updated_at before update on subscriptions for each row execute function set_updated_at();

create table paddle_events (
  id           text primary key,     -- Paddle's event_id; the dedupe key
  type         text not null,
  occurred_at  timestamptz not null,
  received_at  timestamptz not null default now(),
  processed_at timestamptz,
  error        text
);
create index paddle_events_received on paddle_events(received_at desc);
drop table stripe_events;

-- One egress overage line per subscription per period, recorded before the
-- Paddle one-time charge is sent and completed with its transaction id, so
-- a retry never sends it twice.
create table overage_charges (
  subscription_id       text not null references subscriptions(id),
  period_start          timestamptz not null,
  egress_gb             numeric not null,
  cents                 bigint not null,
  paddle_transaction_id text,
  created_at            timestamptz not null default now(),
  primary key (subscription_id, period_start)
);

-- The waitlist gates checkout and invites instead of admitting (I-290).
-- invited_at/hold_until is the 72-hour seat hold; converted_at is set when
-- the invited user's subscription arrives; an expired hold moves the user
-- to the back and counts here.
alter table waitlist rename column admitted_at to invited_at;
alter table waitlist rename column admitted_by to invited_by;
alter table waitlist
  add column hold_until      timestamptz,
  add column converted_at    timestamptz,
  add column expired_invites integer not null default 0;
update waitlist set hold_until = invited_at + interval '72 hours' where invited_at is not null;
drop index if exists waitlist_waiting;
drop index if exists waitlist_admitted;
create index waitlist_waiting on waitlist(joined_at, user_id) where invited_at is null;
create index waitlist_holding on waitlist(hold_until) where invited_at is not null and converted_at is null;

-- Compute is no longer priced per hour; rows from here on carry plan-v1.
alter table usage_hours alter column price_version set default 'plan-v1';
