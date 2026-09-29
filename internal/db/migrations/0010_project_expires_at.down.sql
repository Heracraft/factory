-- Reverts 0010: no project is temporary any more.
drop index if exists projects_expires_at;
alter table projects drop column expires_at;
