-- Temporary machines (DECISIONS I-347, I-349): a project with expires_at
-- set is destroyed by the api's reaper once that time has passed, with no
-- snapshot kept. `repose keep` sets it back to null. Only temporary
-- projects have it, so the reaper's index covers only those rows.
alter table projects add column expires_at timestamptz;
create index projects_expires_at on projects(expires_at) where expires_at is not null and destroyed_at is null;
