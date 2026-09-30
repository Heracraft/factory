-- Reverts 0011. Fails while any subscription is on plus or has a plus
-- downgrade scheduled; move those first, since no older plan means the same.
alter table subscriptions drop constraint subscriptions_plan_check;
alter table subscriptions add constraint subscriptions_plan_check check (plan in ('solo','pro'));
alter table subscriptions drop constraint subscriptions_scheduled_plan_check;
alter table subscriptions add constraint subscriptions_scheduled_plan_check check (scheduled_plan in ('solo','pro'));
