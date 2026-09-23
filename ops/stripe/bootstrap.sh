#!/usr/bin/env bash
# Creates, or finds, every Stripe object the api needs and prints the env
# block to paste into the api's Coolify environment (DECISIONS I-180,
# docs/ops/AZURE-SETUP.md step 17, docs/ops/M4-GATE.md).
#
#   ops/stripe/bootstrap.sh                   # prompts for the key, test mode
#   STRIPE_SECRET_KEY=sk_test_... ops/stripe/bootstrap.sh > /tmp/stripe.env
#   ops/stripe/bootstrap.sh --rotate-webhook  # new endpoint, new signing secret
#   ops/stripe/bootstrap.sh --live            # live mode, on purpose only
#
# It is `repose-admin billing stripe-bootstrap`, run from this checkout with
# `go run` when no repose-admin is on PATH. Progress goes to stderr and the
# block alone to stdout. Rerunning is safe: the product, the three meters,
# the three prices, the portal configuration and the webhook endpoint are
# each found before anything is created, and a second run creates nothing.
# The key is taken from the environment or a silent prompt, never from the
# command line, where it would land in shell history and `ps`.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/../.." && pwd)"

if [[ -z "${STRIPE_SECRET_KEY:-}" ]]; then
	if [[ -t 0 ]]; then
		read -r -s -p "Stripe secret key (sk_test_...): " STRIPE_SECRET_KEY
		echo >&2
	else
		echo "STRIPE_SECRET_KEY is not set and there is no terminal to ask on" >&2
		exit 2
	fi
fi
export STRIPE_SECRET_KEY

if command -v repose-admin >/dev/null 2>&1; then
	exec repose-admin billing stripe-bootstrap "$@"
fi
cd "$root"
exec go run ./cmd/repose-admin billing stripe-bootstrap "$@"
