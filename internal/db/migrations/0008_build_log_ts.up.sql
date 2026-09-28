-- When each build log line reached the api (DECISIONS I-322), so
-- `repose logs --kind build` can print a time for it and `--follow` has a
-- cursor. Rows written before this migration get the time it ran; the
-- default also covers an older api still inserting without the column.
alter table build_logs add column ts timestamptz not null default now();
