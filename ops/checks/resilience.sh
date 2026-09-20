#!/usr/bin/env bash
# M3 step 5 (docs/workstreams/PROMPTS.md "M3 bring-up / m3"): the api's
# resilience rows of 05 §9 on the real path.
#
#   grpc    kill api-grpc during a build: hostd reconnects, the op completes,
#           nothing is lost ("api-grpc restarts are absorbed by hostd
#           reconnect with no lost commands").
#   http    restart api during a snapshot op: the op completes ("ops survive
#           an api restart"; the op driver lives in api-grpc, I-42, so this
#           also proves the http replica is stateless).
#   bump    publish a base as a security release: the unheld project builds
#           on it, the held one does not ("base bump job builds every unheld
#           project and skips held ones"); with a BASE_REV whose kernel
#           differs, `kernel_changed` is true and the project ends
#           `needs_reboot` (12 §9 kernel_changed row).
#   expiry  age two snapshots past 7 days: the expiry run at api-grpc start
#           deletes the older one and never the one a restore is reading
#           ("snapshot expiry removes blobs on schedule and never one
#           referenced by a running restore").
#
# `docker kill` and `docker restart` of the api containers are the only
# state changes this makes on the control VM (never Coolify's UI). Tell the
# conductor before running grpc or http: m3-web may be mid-deploy.
#
#   ops/checks/resilience.sh all|grpc|http|bump|expiry
set -euo pipefail
check=resilience
# shellcheck source-path=SCRIPTDIR
. "$(dirname "$0")/lib.sh"
frags="$checks_dir/fragments"
what=${1:-all}

pid=$(project_id)
containers
log "project $pid; sections: $what"

# hostd's stream events on the host since a time.
stream_events() { host_sh "journalctl -u hostd --since '$1' --no-pager -o cat | grep -E '\"event\":\"stream_(connect|disconnect)\"' | tail -6"; }

grpc_section() {
	[ "$(project_state)" = running ] || fail "$PROJECT is not running"
	local since op
	since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	# Alternate between two fragments so there is always something to build.
	if psql_q "select fragment from config_revisions where id=(select config_revision_id from projects where id='$pid')" | grep -q cowsay; then
		f="$frags/fetchurl-ok.nix"
	else
		f="$frags/package.nix"
	fi
	op=$(api_body PUT "/projects/$pid/config" "{\"fragment\": $(python3 -c 'import json,sys; print(json.dumps(open(sys.argv[1]).read()))' "$f")}" | jsonq 'd["op_id"]')
	log "build op $op started; killing api-grpc in 8 s"
	sleep 8
	control_sh "docker kill $API_GRPC_CTR; sleep 3; docker ps --format '{{.Names}} {{.Status}}' | grep $API_GRPC_CTR || echo 'api-grpc not back yet'" | evidence "docker kill api-grpc during op $op"
	control_sh "for i in \$(seq 1 30); do docker ps --format '{{.Names}} {{.Status}}' | grep -q \"$API_GRPC_CTR.*Up\" && break; sleep 2; done; docker ps --format '{{.Names}} {{.Status}}' | grep $API_GRPC_CTR" | evidence "api-grpc back (Docker's restart policy; if not, redeploy through the conductor)"
	wait_op "$pid" "$op" 1800 || fail "the build op did not complete after the api-grpc kill"
	stream_events "$since" | evidence "hostd stream_disconnect / stream_connect on the host"
	admin ops log "$op" | tail -15 | evidence "repose-admin ops log $op"
	psql_q "select count(*) from ops where project_id='$pid' and state in ('pending','running')" | evidence "ops still pending or running for the project (want 0)"
}

http_section() {
	[ "$(project_state)" = running ] || fail "$PROJECT is not running"
	local op
	op=$(api_body POST "/projects/$pid/snapshots" | jsonq 'd["op_id"]')
	log "snapshot op $op started; restarting api in 3 s"
	sleep 3
	control_sh "docker restart $API_CTR; docker ps --format '{{.Names}} {{.Status}}' | grep $API_CTR" | evidence "docker restart api during snapshot op $op"
	wait_op "$pid" "$op" 900 || fail "the snapshot op did not complete after the api restart"
	"$REPOSE" snapshots list --project "$PROJECT" 2>&1 | tail -4 | evidence "repose snapshots list after the restart"
}

bump_section() {
	: "${PROJECT_HELD:?set PROJECT_HELD (a second project of the same account) for the bump section}"
	: "${BASE_REV:?set BASE_REV (the git revision to publish) for the bump section}"
	local hid ver since
	hid=$("$REPOSE" status --json --project "$PROJECT_HELD" | jsonq 'd["id"]')
	api_body PATCH "/projects/$hid" '{"hold_base_updates":true}' >/dev/null
	api_body PATCH "/projects/$pid" '{"hold_base_updates":false}' >/dev/null
	{ "$REPOSE" status --project "$PROJECT"; "$REPOSE" status --project "$PROJECT_HELD"; } 2>&1 | evidence "repose status before the bump (base lines)"
	ver="$(date -u +%Y.%m.%d)-m3-$(date -u +%H%M)"
	since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	admin base publish --rev "$BASE_REV" --version "$ver" --changelog "m3 resilience check" --security 2>&1 | evidence "repose-admin base publish $ver ($BASE_REV, security)"
	log "waiting up to 12 min for the security sweep (basebump.Run checks every 10 min)"
	local s op=""
	for ((s = 0; s < 720; s += 15)); do
		op=$(psql_q "select id from ops where project_id='$pid' and kind='build' and created_at >= '$since' order by created_at limit 1")
		[ -n "$op" ] && break
		sleep 15
	done
	[ -n "$op" ] || fail "no build op for $PROJECT within 12 min of the security publish"
	wait_op "$pid" "$op" 1800 || true
	psql_q "select id, kind, state, reboot_required, error::text from ops where project_id in ('$pid','$hid') and created_at >= '$since' order by created_at" | evidence "ops enqueued by the sweep (the held project must have none)"
	psql_q "select project_id, base_version, status, kernel_changed, reboot_required from config_revisions where project_id in ('$pid','$hid') order by created_at desc limit 4" | evidence "config_revisions after the sweep (kernel_changed for a kernel bump)"
	psql_q "select id, kind, summary from events where project_id in ('$pid','$hid') and kind like 'base_%' and ts >= '$since'" | evidence "base_* events"
	{ "$REPOSE" status --project "$PROJECT"; "$REPOSE" status --project "$PROJECT_HELD"; } 2>&1 | evidence "repose status after the bump"
	admin base status "$ver" 2>&1 | evidence "repose-admin base status $ver"
	assert_zero "build ops on the held project" "$(psql_q "select count(*) from ops where project_id='$hid' and kind='build' and created_at >= '$since'")"
	api_body PATCH "/projects/$hid" '{"hold_base_updates":false}' >/dev/null
}

expiry_section() {
	[ "$(project_state)" = running ] || fail "$PROJECT must be running to take snapshots"
	local s1 s2 s3 op
	for i in 1 2 3; do
		op=$(api_body POST "/projects/$pid/snapshots" | jsonq 'd["op_id"]')
		wait_op "$pid" "$op" 900 || fail "manual snapshot $i failed"
	done
	read -r s1 s2 s3 <<<"$(psql_q "select string_agg(id::text, ' ' order by taken_at) from (select id, taken_at from snapshots where project_id='$pid' and deleted_at is null order by taken_at desc limit 3) t")"
	log "snapshots oldest→newest: $s1 $s2 $s3"
	psql_q "update snapshots set taken_at = now() - interval '8 days' where id in ('$s1','$s2')" >/dev/null
	"$REPOSE" stop --no-snapshot --project "$PROJECT" 2>&1 | tail -2 | evidence "repose stop before the restore"
	wait_state stopped 600
	op=$("$REPOSE" snapshots restore "$s2" --project "$PROJECT" 2>&1 | tee /dev/stderr | grep -oE '[0-9a-f-]{36}' | tail -1 || true)
	log "restore of $s2 started (op ${op:-unknown}); restarting api-grpc so expiry runs now"
	sleep 5
	control_sh "docker restart $API_GRPC_CTR" | evidence "docker restart api-grpc (expiry runs at start; the restore op is re-driven)"
	sleep 30
	psql_q "select id, taken_at, deleted_at, restoring_op_id from snapshots where id in ('$s1','$s2','$s3') order by taken_at" | evidence "snapshots during the restore: $s1 must be deleted, $s2 (being restored) must not, $s3 (newest) must not"
	assert_zero "aged snapshot $s1 still live" "$(psql_q "select count(*) from snapshots where id='$s1' and deleted_at is null")"
	assert_eq "restoring snapshot $s2 kept" "$(psql_q "select count(*) from snapshots where id='$s2' and deleted_at is null")" "1"
	wait_state running 1200
	control_sh "docker restart $API_GRPC_CTR" >/dev/null
	sleep 30
	psql_q "select id, taken_at, deleted_at, restoring_op_id from snapshots where id in ('$s1','$s2','$s3') order by taken_at" | evidence "snapshots after the restore finished and a second expiry run: $s2 now deleted, $s3 kept"
	assert_zero "aged snapshot $s2 after its restore" "$(psql_q "select count(*) from snapshots where id='$s2' and deleted_at is null")"
	assert_eq "newest snapshot kept" "$(psql_q "select count(*) from snapshots where id='$s3' and deleted_at is null")" "1"
	docker_logs_since "$API_GRPC_CTR" "$(date -u -d '-15 min' +%Y-%m-%dT%H:%M:%SZ)" | grep -E 'snapshot_(expired|deleted|expiry)' | tail -6 | evidence "api-grpc expiry log lines"
}

case $what in
all) grpc_section; http_section; expiry_section; bump_section ;;
grpc) grpc_section ;;
http) http_section ;;
bump) bump_section ;;
expiry) expiry_section ;;
*) echo "usage: $0 all|grpc|http|bump|expiry" >&2; exit 2 ;;
esac
log "resilience ($what) finished; evidence in $report"
