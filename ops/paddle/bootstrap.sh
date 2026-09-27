#!/usr/bin/env bash
# Creates, or finds, every Paddle object the api needs and prints the
# PADDLE_* block to paste into the api's Coolify environment (DECISIONS
# I-289, docs/ops/AZURE-SETUP.md step 17, docs/ops/M4-GATE.md).
#
#   ops/paddle/bootstrap.sh                    # prompts for the key, sandbox
#   PADDLE_API_KEY=pdl_sdbx_... ops/paddle/bootstrap.sh > /tmp/paddle.env
#   ops/paddle/bootstrap.sh --no-webhook       # catalog only
#   ops/paddle/bootstrap.sh --live             # live, on purpose only
#
# It is `repose-admin billing paddle-bootstrap`, run from this checkout with
# `go run` when no repose-admin is on PATH. Progress goes to stderr and the
# block alone to stdout. Rerunning is safe: the three products, the two
# prices and the notification destination are each found (by
# custom_data.repose and by destination URL) before anything is created,
# and a second run creates nothing. The key is taken from the environment
# or a silent prompt, never from the command line, where it would land in
# shell history and `ps`.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/../.." && pwd)"

if [[ -z "${PADDLE_API_KEY:-}" ]]; then
	if [[ -t 0 ]]; then
		read -r -s -p "Paddle API key (pdl_sdbx_...): " PADDLE_API_KEY
		echo >&2
	else
		echo "PADDLE_API_KEY is not set and there is no terminal to ask on" >&2
		exit 2
	fi
fi
export PADDLE_API_KEY

if command -v repose-admin >/dev/null 2>&1; then
	exec repose-admin billing paddle-bootstrap "$@"
fi
cd "$root"
exec go run ./cmd/repose-admin billing paddle-bootstrap "$@"
