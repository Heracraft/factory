-- Reverts 0003_billing. usage_hours keeps its rows and its cost columns;
-- only the billing-period columns this migration added go away, so a
-- rollback loses the period attribution and the credit ledger, not the
-- meter history (09-billing.md §8).
alter table users
  drop column if exists past_due_since,
  drop column if exists billing_anchor,
  drop column if exists stripe_subscription_id;

drop index if exists usage_hours_period;
drop index if exists usage_hours_push_pending;
alter table usage_hours
  drop column if exists price_version,
  drop column if exists credit_cents,
  drop column if exists period_end,
  drop column if exists period_start;

drop table if exists stripe_events;

drop trigger if exists credit_ledger_apply on credit_ledger;
drop function if exists credit_ledger_apply();
drop table if exists credit_ledger;
