#!/usr/bin/env bash
# The dashboard's design preview: cmd/fakeapi with seeded projects behind
# the Vite dev server, reachable on this box's Tailscale address. Sign-in is
# the dev-only preview login (PUBLIC_PREVIEW_TOKEN, src/lib/auth.svelte.ts):
# "Sign in" on the landing page opens the dashboard without Logto.
#
#   ops/dev/web-preview.sh            # port 5173
#   PORT=5180 ops/dev/web-preview.sh
set -euo pipefail
cd "$(dirname "$0")/../.."
PORT="${PORT:-5173}"
TOKEN=preview
BIN="${TMPDIR:-/tmp}/repose-fakeapi"

go build -o "$BIN" ./cmd/fakeapi
coproc FAKE { "$BIN" --billing; }
read -r line <&"${FAKE[0]}"; API="${line#FAKEAPI_URL=}"       # http://127.0.0.1:N/v1
read -r line <&"${FAKE[0]}"; ADMIN="${line#FAKEAPI_ADMIN_URL=}"
trap 'kill "$FAKE_PID" 2>/dev/null || true' EXIT

curl -fsS -X POST "$ADMIN/billing" -d '{"mode":"card"}' >/dev/null
api() { curl -fsS -X "$1" "$API$2" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' ${3:+-d "$3"}; }
id_of() { python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])'; }

izma=$(api POST /projects '{"name":"izma","remote_url":"github.com/teksafari/izma","class":"large","tz":"Africa/Nairobi"}' | id_of)
api POST /projects '{"name":"teksafari-org","remote_url":"github.com/teksafari/teksafari.org","class":"large"}' >/dev/null
nuru=$(api POST /projects '{"name":"nuru-playground","remote_url":"github.com/nuruprogramming/nuru","class":"small"}' | id_of)
api POST /projects '{"name":"recruiting","remote_url":"github.com/heracraft/recruiting","class":"xl"}' >/dev/null
old=$(api POST /projects '{"name":"age-calculator","remote_url":"github.com/heracraft/age-calculator","class":"small"}' | id_of)
api POST "/projects/$izma/snapshots" '{}' >/dev/null || true
api POST "/projects/$nuru/stop" '{"snapshot":true}' >/dev/null || true
api DELETE "/projects/$old" >/dev/null || true

IP="$(tailscale ip -4 2>/dev/null | head -1 || echo 127.0.0.1)"
echo "Preview: http://$IP:$PORT  (fake api $API)"
cd apps/web
FAKEAPI_TARGET="${API%/v1}" PUBLIC_API_URL=/fakeapi/v1 PUBLIC_PREVIEW_TOKEN=$TOKEN \
	PUBLIC_LOGTO_ENDPOINT=http://127.0.0.1:1 PUBLIC_LOGTO_APP_ID=preview \
	pnpm exec vite dev --host 0.0.0.0 --port "$PORT" --strictPort
