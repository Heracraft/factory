-- Reverts 0004_gateway_session_id. Rows a new gateway wrote (one per
-- relay) cannot fit the old key, so they go; the table only feeds the
-- displayed session count, and open sessions are reported again on their
-- next open.
delete from gateway_sessions where session_id <> '';
alter table gateway_sessions drop constraint gateway_sessions_pkey;
alter table gateway_sessions drop column session_id;
alter table gateway_sessions add primary key (project_id, cert_serial);
