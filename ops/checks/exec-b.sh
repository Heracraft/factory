#!/usr/bin/env bash
# The EXEC_B prefix for test/isolation when tenant B has no certificate of
# its own: runs the script through `repose-admin exec` (audited, as dev in
# the guest) and prints only the command's own output. repose-admin prints
# "op <id> enqueued" on stdout before the output and reports the exit code
# in words; this turns both back into what a plain ssh would give.
#
#   exec-b.sh <control ip> <api container> <project id> <script>
set -uo pipefail
control=$1 ctr=$2 pid=$3 script=$4
out=$(ssh -o BatchMode=yes -o ConnectTimeout=20 "root@$control" "docker exec -i $ctr /usr/local/bin/repose-admin exec $pid -- sh -c $(printf '%q' "$script")" 2>&1)
rc=$?
printf '%s\n' "$out" | grep -vE '^op [0-9a-f-]{36} enqueued$'
exit $rc
