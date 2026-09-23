-- A gateway session is one relay, not one certificate (DECISIONS I-176).
-- Keyed (project_id, cert_serial), two connections under one certificate
-- (two terminals, `repose open` beside an attach) were one row, and the
-- first to close deleted it while the other was still open. The gateway
-- now sends a per-relay session_id; a report without one (a gateway older
-- than this, accepted for one release) keeps the old one-row-per-cert
-- behaviour under session_id ''.
alter table gateway_sessions add column session_id text not null default '';
alter table gateway_sessions drop constraint gateway_sessions_pkey;
alter table gateway_sessions add primary key (project_id, cert_serial, session_id);
