#!/usr/bin/env bash
# Push a few log lines of the documented shape into the dev stack's Loki, so
# the two Loki panels (Builds "Recent build failures", Gateway "Recent
# authentication failures") have something to render.
#
#   docker compose -f ops/dev/docker-compose.yml up -d loki
#   ops/dev/seedlogs.sh
#
# The lines are what internal/obs writes: ts, level, component, event, msg and
# bounded context fields, and nothing from the never-log list. On a host the
# same lines arrive through Fluent Bit with the labels
# docs/ops/OBSERVABILITY.md documents.
set -euo pipefail

LOKI=${LOKI:-http://127.0.0.1:3100}

python3 - "$LOKI" <<'PY'
import json, sys, time, urllib.request

loki = sys.argv[1]
now = int(time.time() * 1e9)


def at(seconds_ago, obj):
    return [str(now - seconds_ago * 1_000_000_000), json.dumps(obj)]


streams = [
    {
        "stream": {"component": "hostd", "host": "dev-host"},
        "values": [
            at(120, {"level": "INFO", "msg": "build start", "component": "hostd",
                     "event": "build_start", "project_id": "p-1", "revision_id": "r-9"}),
            at(90, {"level": "WARN", "msg": "build failed", "component": "hostd",
                    "event": "build_fail", "code": "eval_failed", "project_id": "p-1",
                    "revision_id": "r-9", "duration_ms": 4200}),
            at(60, {"level": "WARN", "msg": "build failed", "component": "hostd",
                    "event": "build_fail", "code": "closure_too_large", "project_id": "p-2",
                    "revision_id": "r-10", "duration_ms": 812000}),
            at(30, {"level": "INFO", "msg": "build done", "component": "hostd",
                    "event": "build_done", "project_id": "p-3", "duration_ms": 61000,
                    "eval_ms": 5200, "build_ms": 55000, "closure_bytes": 3221225472}),
        ],
    },
    {
        "stream": {"component": "gateway", "host": "edge"},
        "values": [
            at(100, {"level": "WARN", "msg": "auth failed", "component": "gateway",
                     "event": "auth_fail", "reason": "expired", "project_id": "p-1",
                     "cert_serial": "41"}),
            at(45, {"level": "WARN", "msg": "auth failed", "component": "gateway",
                    "event": "auth_fail", "reason": "bad_cert", "cert_serial": "0"}),
            at(20, {"level": "INFO", "msg": "session open", "component": "gateway",
                    "event": "session_open", "project_id": "p-1"}),
        ],
    },
    {
        "stream": {"component": "console", "guest_id": "g-dev-1", "host": "dev-host"},
        "values": [
            at(70, {"line": "systemd[1]: Reached target Multi-User System."}),
            at(50, {"line": "dev: npm run test"}),
        ],
    },
]

req = urllib.request.Request(
    loki + "/loki/api/v1/push",
    data=json.dumps({"streams": streams}).encode(),
    headers={"Content-Type": "application/json"},
)
with urllib.request.urlopen(req) as r:
    print("push:", r.status)
PY

echo "pushed. Check with:"
echo "  curl -s '$LOKI/loki/api/v1/query_range?query=%7Bcomponent%3D%22hostd%22%7D&limit=5' | head -c 400"
