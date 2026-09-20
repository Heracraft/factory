#!/usr/bin/env bash
# M3 step 6 (docs/workstreams/PROMPTS.md "M3 bring-up / m3"): the boundary
# table of docs/workstreams/14-security.md §5 as a test on host-01, with
# two guests of two users. It fills the REPOSE_ISOLATION_* environment
# test/isolation/doc.go lists and runs the Go suite; a row whose inputs are
# missing skips by name, so read the summary, not the exit code alone.
#
# Where the scripts run:
#   guest A (the owner's PROJECT)   through the gateway, as the user, with
#                                   the certificate `repose run` wrote
#   guest B (PROJECT_B, the second  `repose-admin exec` (audited, as dev);
#   person's project)               the second person's own certificate is
#                                   only needed by the gateway rows, as
#                                   LOGIN_B, and never leaves their laptop
#   the host                        root over the edge's operator sshd
#
# Also here, because the Go suite has no row for them: the operator password
# attempt (14 §9 "a password attempt is logged"), and the note that "hostd for
# host X cannot act on host Y" needs a second host.
#
#   ops/checks/isolation-host01.sh    # PROJECT and PROJECT_B running
set -euo pipefail
check=isolation
# shellcheck disable=SC1091
. "$(dirname "$0")/lib.sh"
: "${PROJECT_B:?set PROJECT_B (the second user project slug) in the environment}"
: "${LOGIN_B:?set LOGIN_B (<slug>.<handle> of the second user project)}"
containers

# Ids and addresses from repose-admin (the api's view) and the host.
a=$(admin projects show "$PROJECT")
b=$(admin projects show "$PROJECT_B")
field() { printf '%s\n' "$1" | awk -v k="$2" '$1==k{print $2}'; }
A_PID=$(field "$a" id); A_GID=$(field "$a" guest_id); A_IP=$(field "$a" guest_ip)
B_PID=$(field "$b" id); B_GID=$(field "$b" guest_id); B_IP=$(field "$b" guest_ip)
[ "$(field "$a" user)" != "$(field "$b" user)" ] || fail "PROJECT and PROJECT_B belong to the same user; the rows need two tenants"
hostinfo=$(host_sh "jq -r .host_id /var/lib/repose/hostd/host.json; . /run/repose/host.env; echo \$BRIDGE_ADDR; echo \$GUEST_CIDR; jq -r .mac /var/lib/repose/guests/$A_GID/guest.json")
HOST_ID=$(printf '%s\n' "$hostinfo" | sed -n 1p)
HOST_IP=$(printf '%s\n' "$hostinfo" | sed -n 2p | cut -d/ -f1)
GUEST_CIDR=$(printf '%s\n' "$hostinfo" | sed -n 3p)
A_MAC=$(printf '%s\n' "$hostinfo" | sed -n 4p)
B_TAP="tap-${B_GID:0:8}"
# An address in a /22 that is not this host's: the next /22 up.
OTHER_GUEST_IP=$(python3 -c 'import ipaddress,sys; n=ipaddress.ip_network(sys.argv[1]); print(n.broadcast_address+2)' "$GUEST_CIDR")
GATEWAY="$(awk '/^ *HostName/{print $2; exit}' ~/.ssh/repose/config):22"
LOGIN_A=$(awk -v h="Host $PROJECT.repose" '$0==h{f=1} f&&/^ *User/{print $2; exit}' ~/.ssh/repose/config)
{
	echo "host $HOST_ID bridge $HOST_IP cidr $GUEST_CIDR other-range probe $OTHER_GUEST_IP"
	echo "A: $PROJECT $A_PID guest $A_GID ip $A_IP mac $A_MAC login $LOGIN_A"
	echo "B: $PROJECT_B $B_PID guest $B_GID ip $B_IP tap $B_TAP login $LOGIN_B"
} | evidence "inputs"

export REPOSE_ISOLATION_HOST_ID="$HOST_ID"
export REPOSE_ISOLATION_EXEC_A="ssh -o BatchMode=yes -o ConnectTimeout=20 $PROJECT.repose"
export REPOSE_ISOLATION_EXEC_B="ssh -o BatchMode=yes -o ConnectTimeout=20 root@$CONTROL_IP docker exec -i $API_CTR /usr/local/bin/repose-admin exec $B_PID -- sh -c"
export REPOSE_ISOLATION_HOST_EXEC="ssh -o BatchMode=yes -o ConnectTimeout=20 -J root@$EDGE:$EDGE_SSH_PORT root@$HOST_WG_IP"
export REPOSE_ISOLATION_A_IP="$A_IP" REPOSE_ISOLATION_B_IP="$B_IP" REPOSE_ISOLATION_A_MAC="$A_MAC"
export REPOSE_ISOLATION_B_TAP="$B_TAP" REPOSE_ISOLATION_A_GUEST_ID="$A_GID" REPOSE_ISOLATION_A_SLUG="$PROJECT"
export REPOSE_ISOLATION_HOST_IP="$HOST_IP" REPOSE_ISOLATION_OTHER_GUEST_IP="$OTHER_GUEST_IP"
export REPOSE_ISOLATION_GATEWAY="$GATEWAY"
export REPOSE_ISOLATION_KEY_A="$HOME/.ssh/id_ed25519" REPOSE_ISOLATION_CERT_A="$HOME/.ssh/repose/id_ed25519-cert.pub"
export REPOSE_ISOLATION_LOGIN_A="$LOGIN_A" REPOSE_ISOLATION_LOGIN_B="$LOGIN_B"
tok=$(token)
export REPOSE_ISOLATION_API_URL="$API_URL" REPOSE_ISOLATION_TOKEN_A="$tok"
export REPOSE_ISOLATION_PROJECT_A="$A_PID" REPOSE_ISOLATION_PROJECT_B="$B_PID"
[ -n "${LOKI_URL:-}" ] && export REPOSE_ISOLATION_LOKI_URL="$LOKI_URL"
# DIRECT_SSH stays unset: it would put the owner's private key on the host.

# TestRevokedCertificateRejected revokes CERT_A; `repose run` reissues one
# afterwards, so it runs last and the order is pinned here.
root=$(cd "$checks_dir/../.." && pwd)
(cd "$root" && go test ./test/isolation/ -run . -v -count=1 2>&1) | tee "$OUT/isolation-go-$stamp.txt" | grep -E '^(=== RUN|--- (PASS|FAIL|SKIP)|PASS|FAIL|ok|\s+.*_test.go)' | evidence "go test ./test/isolation/ against host $HOST_ID (full output: $OUT/isolation-go-$stamp.txt)"
grep -E '^--- (PASS|FAIL|SKIP)' "$OUT/isolation-go-$stamp.txt" | sort | uniq -c | evidence "row summary"

# Operator access: a password attempt is refused and logged, on the host
# and on the edge (14 §9). Certificate-only is the design; today the host
# admits the bootstrap key (I-92) and the operator key on the edge.
since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
set +e
ssh -o BatchMode=yes -o ConnectTimeout=20 -o PubkeyAuthentication=no -o PreferredAuthentications=password -J "root@$EDGE:$EDGE_SSH_PORT" "root@$HOST_WG_IP" true 2>&1 | tail -2
echo "host password attempt exit: ${PIPESTATUS[0]}"
ssh -o BatchMode=yes -o ConnectTimeout=20 -o PubkeyAuthentication=no -o PreferredAuthentications=password -p "$EDGE_SSH_PORT" "root@$EDGE" true 2>&1 | tail -2
echo "edge password attempt exit: ${PIPESTATUS[0]}"
set -e
sleep 2
host_sh "journalctl -u sshd --since '$since' --no-pager -o cat | grep -iE 'password|no supported authentication|Connection closed by authenticating' | tail -5" | evidence "host sshd journal for the password attempt"
ssh -o BatchMode=yes -p "$EDGE_SSH_PORT" "root@$EDGE" "journalctl -u sshd --since '$since' --no-pager -o cat | grep -iE 'password|no supported authentication|Connection closed by authenticating' | tail -5" | evidence "edge sshd journal for the password attempt"
ssh -o BatchMode=yes -p "$EDGE_SSH_PORT" "root@$EDGE" "sshd -T 2>/dev/null | grep -E '^(passwordauthentication|kbdinteractiveauthentication|permitrootlogin) '" | evidence "edge sshd effective settings"
host_sh "sshd -T -f /etc/ssh/sshd_config 2>/dev/null | grep -E '^(passwordauthentication|kbdinteractiveauthentication|permitrootlogin|trustedusercakeys) '" | evidence "host sshd effective settings"

cat <<EOT | evidence "rows this script cannot run"
hostd for host X cannot act on host Y: one host exists (host-01); the row needs a second registered host. The api's per-stream check is TestRegisterSessionSendSweep locally.
DIRECT_SSH (B's sshd refusing A's certificate from the host): not run, it would copy the owner's private key to the host; the gateway half of the row ran above.
EOT
log "isolation check finished; evidence in $report"
