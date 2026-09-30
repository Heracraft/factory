-- A third plan (DECISIONS I-362): Plus is what Pro was (16 GB, $59) and
-- Pro is the new top plan (32 GB, $99). The ids stay text; only the check
-- constraints learn 'plus'.
alter table subscriptions drop constraint subscriptions_plan_check;
alter table subscriptions add constraint subscriptions_plan_check check (plan in ('solo','plus','pro'));
alter table subscriptions drop constraint subscriptions_scheduled_plan_check;
alter table subscriptions add constraint subscriptions_scheduled_plan_check check (scheduled_plan in ('solo','plus','pro'));
