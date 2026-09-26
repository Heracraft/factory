-- Reverts 0007: user-scoped events (waitlist admissions) go with the
-- column that made them possible.
delete from events_outbox where event_id in (select id from events where project_id is null);
delete from events where project_id is null;
drop index if exists events_user;
alter table events drop constraint events_owner;
alter table events drop column user_id;
alter table events alter column project_id set not null;
drop table waitlist;
