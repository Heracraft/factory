#!/usr/bin/env bash
# claude-auth-trace.sh: the release check for the Claude login share
# (DECISIONS I-278). The share works because Claude Code, when renaming its
# new .credentials.json over the old one fails with EBUSY (the file is a
# bind mount), rewrites the file in place. That fallback is not documented;
# a Claude Code version that drops it fails every token refresh on the
# share. Run this in an e2e guest on the base about to be published,
# signed in (one /login in any of the owner's machines), before publish:
#
#   scp ops/dev/claude-auth-trace.sh e2e-x.repose: && ssh e2e-x.repose ./claude-auth-trace.sh
#
# It forces a refresh (expiresAt in the past, rewritten in place so the
# mount stays), runs one prompt under strace, and passes only when the
# rename was refused, the file was rewritten in place, the inode is the
# same and expiresAt moved forward. Prints syscalls, inodes and expiresAt
# only, never a token. Exit 0 pass, 1 fail, 2 cannot run.
set -u
C=$HOME/.claude/.credentials.json
OUT=$(mktemp -d)
trap 'rm -rf "$OUT"' EXIT

command -v strace >/dev/null || { echo "strace missing"; exit 2; }
findmnt -n --mountpoint "$C" >/dev/null || { echo "$C is not the login share (no bind mount)"; exit 2; }
jq -e .claudeAiOauth.refreshToken "$C" >/dev/null 2>&1 || { echo "not signed in: run claude and /login first"; exit 2; }

echo "claude $(claude --version)"
ino=$(stat -c %i "$C")
t=$(mktemp) && jq '.claudeAiOauth.expiresAt = 1000' "$C" > "$t" && cat "$t" > "$C" && rm -f "$t"
strace -f -qq -e trace=openat,rename,renameat,renameat2,unlink,unlinkat,chmod -o "$OUT/trace" \
  claude -p "reply with the single word ok" < /dev/null > "$OUT/out" 2>&1
rc=$?
grep -F '.credentials.json' "$OUT/trace" | grep -v O_RDONLY | sed -E 's/^[0-9]+ +//'

fail=0
check() { if "$@" >/dev/null 2>&1; then echo "ok   $desc"; else echo "FAIL $desc"; fail=1; fi; }
desc="prompt answered (claude rc=$rc)"; check test "$rc" -eq 0
desc="rename over the file refused with EBUSY"; check grep -qE 'rename.*credentials\.json.*EBUSY' "$OUT/trace"
desc="file rewritten in place"; check grep -qE 'openat\(.*credentials\.json", O_WRONLY' "$OUT/trace"
desc="same inode ($ino)"; check test "$(stat -c %i "$C")" = "$ino"
desc="expiresAt moved forward"; check test "$(jq -r .claudeAiOauth.expiresAt "$C")" -gt 1000
exit $fail
