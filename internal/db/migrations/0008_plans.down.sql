alter table usage_hours alter column price_version set default 'v1';
drop index if exists waitlist_holding;
drop index if exists waitlist_waiting;
alter table waitlist drop column expired_invites, drop column converted_at, drop column hold_until;
alter table waitlist rename column invited_by to admitted_by;
alter table waitlist rename column invited_at to admitted_at;
create index waitlist_waiting on waitlist(joined_at, user_id) where admitted_at is null;
create index waitlist_admitted on waitlist(admitted_at) where admitted_at is not null;
drop table overage_charges;
create table stripe_events (
  id           text primary key,
  type         text not null,
  received_at  timestamptz not null default now(),
  processed_at timestamptz,
  error        text
);
create index stripe_events_received on stripe_events(received_at desc);
drop table paddle_events;
drop table subscriptions;
alter table users alter column trial_credit_cents set default 1000;
alter table users alter column billing_status set default 'trial';
update users set billing_status = 'trial' where billing_status = 'none';
alter table users drop constraint users_billing_status_check;
alter table users add constraint users_billing_status_check
  check (billing_status in ('trial','active','past_due','suspended','exempt'));
alter table users add column stripe_subscription_id text unique;
alter table users rename column paddle_customer_id to stripe_customer_id;
