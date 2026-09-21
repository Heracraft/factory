#!/usr/bin/env bash
# 14 §5 "Audit coverage" and its §9 row "Every audited action writes an
# audit_log row: one row per action type, triggered deliberately". This
# triggers what can be triggered without disrupting a tenant, then prints
# the audit_log rows since the start grouped by action, and names the
# actions that have no producer yet so the row is closed honestly.
#
# Actions and how they are triggered here:
#   cert_issue        POST /certs with the CLI's public key
#   cert_revoke       POST /certs/revoke {serial} of that certificate
#   secret_put/delete PUT and DELETE a throwaway secret
#   exec              repose-admin exec (an audited Exec over gRPC and vsock)
#   project_snapshot  repose-admin projects snapshot (a repose-admin command)
#   host_add, host_drain  not triggered (a token minted for nothing; a drain
#                     refuses placements on a shared host for its seconds)
#   user_suspend/unsuspend  not triggered: it stops every guest of the user
#   project_restore   the admin path is audited (`repose-admin projects
#                     restore`); the user route POST .../restore is not
#                     (finding, below); resilience.sh expiry exercises the
#                     user route
#   operator login    `hostd audit-login` writes a journal line on the host
#                     (host-conventions.md: "or a journal line until the api
#                     exists"); no api ingest exists, so there is no
#                     audit_log row for it (finding, below)
#
#   ops/checks/audit-rows.sh           # PROJECT running
set -euo pipefail
check=audit
# shellcheck source-path=SCRIPTDIR
. "$(dirname "$0")/lib.sh"
since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
pid=$(project_id)
containers

# cert_issue and cert_revoke
pub=$(cat ~/.ssh/id_ed25519.pub)
cert=$(api_body POST /certs "{\"public_key\": $(printf '%s' "$pub" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read().strip()))'), \"project_ids\": [\"$pid\"]}")
certfile=$(mktemp)
printf '%s' "$cert" | jsonq 'd["certificate"]' >"$certfile"
serial=$(ssh-keygen -L -f "$certfile" | awk '$1=="Serial:"{print $2}')
rm -f "$certfile"
log "issued certificate serial $serial"
api_body POST /certs/revoke "{\"serial\": $serial}" >/dev/null
# secret_put and secret_delete
M3_AUDIT_PROBE="audit-probe-$(openssl rand -hex 4)"
export M3_AUDIT_PROBE
"$REPOSE" secrets set M3_AUDIT_PROBE --from-env --project "$PROJECT" >/dev/null
"$REPOSE" secrets rm M3_AUDIT_PROBE --project "$PROJECT" >/dev/null
unset M3_AUDIT_PROBE
# exec (gRPC Exec with an audit id, guestd Exec as dev); the secrets rm
# above is an op that must finish first (I-70)
wait_idle "$pid"
admin exec "$pid" -- id -un | evidence "repose-admin exec $pid -- id -un"
# a repose-admin command on the project (a host drain would be another,
# but it refuses placements for the seconds it lasts, which is not a
# thing to do on a shared host while someone may be creating a guest)
admin projects snapshot "$pid" | tail -1 | evidence "repose-admin projects snapshot $pid"
# an operator login on the host: the journal line, and its absence in audit_log
host_sh "journalctl -t hostd-audit --since '-5 min' --no-pager -o cat | tail -2" | evidence "hostd audit-login journal lines on the host (the login this script's own ssh made)"

sleep 3
psql_q "select action, count(*), min(actor), min(target) from audit_log where ts >= '$since' group by action order by action" | evidence "audit_log rows since $since, by action"
admin audit --since 1h | head -40 | evidence "repose-admin audit --since 1h (head)"
for a in cert_issue cert_revoke secret_put secret_delete exec project_snapshot; do
	n=$(psql_q "select count(*) from audit_log where ts >= '$since' and action='$a'")
	[ "$n" != "0" ] || fail "no audit_log row for $a"
	log "ok: $a: $n row(s)"
done
assert_zero "the exec argv in hostd's journal (I-53: audit_id and length only)" "$(host_sh "journalctl -u hostd --since '$since' --no-pager -o cat | grep -c '\"argv\":' || true")"

cat <<EOT | evidence "audited actions with no audit_log producer (14 §5 list against the code, 2026-09-20)"
operator SSH login to a host or the edge: journal line only (hostd audit-login, event operator_login); no api ingest, no audit_log row.
restore through the user route (POST /projects/:id/snapshots/:sid/restore): no audit row; only repose-admin projects restore writes project_restore.
user suspension: producer exists (user_suspend); not triggered here because it stops the user's guests.
EOT
log "audit check finished; evidence in $report"
