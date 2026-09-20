#!/usr/bin/env bash
# M3 step 2 (docs/workstreams/PROMPTS.md "M3 bring-up / m3"): a named secret
# set through the api lands in the guest's tmpfs with the documented mode,
# is exported into login shells, is removed when deleted, is refused inside
# a fragment, and its value appears in no log, no route and no audit row.
#
# Closes, with the evidence file it writes: docs/features/secrets.md "Kind 3"
# rules on the real path; 05 §9 "No route ever returns a secret value" (the
# real-path half); 14 §5 rows "A user cannot read secret values" and
# "every secret set or delete (name only)"; the M3 gate line "secrets set in
# the dashboard appear in a guest" (the dashboard calls the same routes).
#
#   ops/checks/secrets.sh            # PROJECT must be running
set -euo pipefail
check=secrets
# shellcheck disable=SC1091
. "$(dirname "$0")/lib.sh"

NAME=M3_CHECK_SECRET
VALUE="m3s-$(openssl rand -hex 12)"
since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
pid=$(project_id)
[ "$(project_state)" = running ] || fail "$PROJECT is $(project_state), not running"
gid=$(guest_id)
log "project $pid guest $gid; secret $NAME (${#VALUE} bytes; the value is never printed)"
containers

# 1. Set it through the api. The CLI reads the value from the environment,
#    so it is on no command line (`ps`, shell history).
export "$NAME=$VALUE"
"$REPOSE" secrets set "$NAME" --from-env --project "$PROJECT" 2>&1 | evidence "repose secrets set $NAME"
unset "$NAME"

# 2. In the guest within 5 s (secrets.md): 0400 dev on a tmpfs in a 0700 dev
#    directory, exported by a login shell, one line in secrets.env.
sleep 5
got=$(guest_sh "$PROJECT" "
stat -c '%U %G %a' /run/repose/secrets/$NAME
stat -c '%U %a' /run/repose/secrets
findmnt -T /run/repose/secrets -no FSTYPE
cat /run/repose/secrets/$NAME; echo
bash -lc 'printf %s \"\$$NAME\"'; echo
grep -c '^export $NAME=' /run/repose/secrets.env
stat -c '%U %a' /run/repose/secrets.env")
printf '%s\n' "$got" | sed "s/$VALUE/<the value>/g" | evidence "in the guest: file owner/mode, dir owner/mode, fstype, content (masked), login-shell export (masked), secrets.env lines, secrets.env mode"
assert_eq "file owner and mode" "$(printf '%s\n' "$got" | sed -n 1p)" "dev dev 400"
assert_eq "directory owner and mode" "$(printf '%s\n' "$got" | sed -n 2p)" "dev 700"
assert_eq "filesystem" "$(printf '%s\n' "$got" | sed -n 3p)" "tmpfs"
assert_eq "file content" "$(printf '%s\n' "$got" | sed -n 4p)" "$VALUE"
assert_eq "login-shell export" "$(printf '%s\n' "$got" | sed -n 5p)" "$VALUE"
assert_eq "secrets.env line" "$(printf '%s\n' "$got" | sed -n 6p)" "1"
assert_eq "secrets.env owner and mode" "$(printf '%s\n' "$got" | sed -n 7p)" "dev 400"

# 3. The list route carries names and timestamps, nothing else.
list=$(api_body GET "/projects/$pid/secrets")
printf '%s\n' "$list" | evidence "GET /projects/:id/secrets"
assert_zero "a value field in the list" "$(printf '%s' "$list" | grep -c '"value"' || true)"
assert_zero "the value in the list" "$(printf '%s' "$list" | grep -c -- "$VALUE" || true)"
"$REPOSE" secrets list --project "$PROJECT" 2>&1 | evidence "repose secrets list"

# 4. A fragment carrying the value is refused before evaluation
#    (secrets.md "Where secrets are not"; locally
#    TestRestoreOntoHostAndSecretValueInFragmentRefused).
frag=$(mktemp)
printf '{ home.file.".m3-leak".text = "%s"; }\n' "$VALUE" >"$frag"
set +e
out=$("$REPOSE" config apply "$frag" --project "$PROJECT" 2>&1)
rc=$?
set -e
rm -f "$frag"
printf '%s\n' "$out" | sed "s/$VALUE/<the value>/g" | evidence "repose config apply of a fragment containing the value (exit $rc)"
[ $rc -ne 0 ] || fail "a fragment containing the secret value was accepted"
printf '%s' "$out" | grep -q "fragment contains the value of secret $NAME" || fail "the refusal did not name the secret"

# 5. Delete: the row is gone, the guest file is gone within 5 s, secrets.env
#    is rewritten (WriteSecrets carries the whole set, DECISIONS I-30).
"$REPOSE" secrets rm "$NAME" --project "$PROJECT" 2>&1 | evidence "repose secrets rm $NAME"
sleep 5
after=$(guest_sh "$PROJECT" "
if [ -e /run/repose/secrets/$NAME ]; then echo present; else echo absent; fi
grep -c '^export $NAME=' /run/repose/secrets.env || true")
printf '%s\n' "$after" | evidence "in the guest after rm: file, secrets.env lines"
assert_eq "file after rm" "$(printf '%s\n' "$after" | sed -n 1p)" "absent"
assert_eq "secrets.env after rm" "$(printf '%s\n' "$after" | sed -n 2p)" "0"
assert_zero "secrets rows after rm" "$(psql_q "select count(*) from secrets where project_id='$pid' and name='$NAME'")"

# 6. The value is nowhere: api and api-grpc logs since the start, hostd's
#    journal on the host, build_logs, events, audit_log. The audit rows for
#    the two actions exist and carry the name only.
assert_zero "the value in api logs" "$(docker_logs_since "$API_CTR" "$since" | grep -c -- "$VALUE" || true)"
assert_zero "the value in api-grpc logs" "$(docker_logs_since "$API_GRPC_CTR" "$since" | grep -c -- "$VALUE" || true)"
assert_zero "the value in hostd's journal" "$(host_sh "journalctl -u hostd --since '$since' --no-pager -o cat | grep -c -- '$VALUE' || true")"
assert_zero "the value in build_logs" "$(psql_q "select count(*) from build_logs where line like '%$VALUE%'")"
assert_zero "the value in events" "$(psql_q "select count(*) from events where summary like '%$VALUE%'")"
assert_zero "the value in audit_log" "$(psql_q "select count(*) from audit_log where detail::text like '%$VALUE%' or target like '%$VALUE%'")"
audit=$(psql_q "select ts, actor, action, target, detail::text from audit_log where action in ('secret_put','secret_delete') and target='$pid' and ts >= '$since' order by ts")
printf '%s\n' "$audit" | evidence "audit_log rows for secret_put and secret_delete on $pid (name only)"
printf '%s' "$audit" | grep -q secret_put || fail "no secret_put audit row"
printf '%s' "$audit" | grep -q secret_delete || fail "no secret_delete audit row"
log "secrets check passed; evidence in $report"
