-- The guest's listening processes from each sample (DECISIONS I-200):
-- [{port, comm, age_seconds, rss_bytes}], so `repose status` can show which
-- dev servers are still up without an SSH connection. Only the newest
-- sample's is ever read. A hostd older than this sends none; the column is
-- then null, which reads as "not known".
alter table meter_samples add column listening jsonb;
