#!/usr/bin/env bash
# SSH tunnels from this machine to the real metrics endpoints, so the local
# stack (ops/dev/docker-compose.yml) can scrape production without being a
# WireGuard peer of the edge.
#
#   ops/dev/tunnel-prod.sh <edge-ip> <control-ip>
#   docker compose -f ops/dev/docker-compose.yml up -d
#   # then point the local Prometheus at ops/dev/prometheus-prod.yml
#
# Why it exists: the supported way to see these numbers is the monitoring
# server's WireGuard peer (ops/prometheus/wireguard-peer.conf), which is the
# owner's machine. An operator with the operator SSH key and no peer — the
# person debugging why the peer does not work, for instance — has this
# instead. It reads; it changes nothing on either machine.
#
# Local ports, matching ops/dev/prometheus-prod.yml:
#   19100 edge node_exporter      19102 gateway
#   19103 api                     19104 api-grpc
#   19200 host node_exporter      19201 hostd     19202 host Fluent Bit
#
# The host's three are forwarded *through* the edge, which reaches them
# over WireGuard. That works whatever the edge's forward chain says,
# because the edge originates the connection itself rather than routing
# somebody else's packet — which is exactly the distinction that makes
# the real monitoring peer need `repose.edge.monitoring.peerCIDRs`
# (DECISIONS I-94). Do not read a working tunnel as a working peer.
#
# Stop it with Ctrl-C, or `ops/dev/tunnel-prod.sh --stop`.
set -euo pipefail

PIDFILE=${TMPDIR:-/tmp}/repose-tunnel-prod.pids
EDGE_SSH_PORT=${EDGE_SSH_PORT:-2222}
# Where the forwarded ports listen. 127.0.0.1 is right for curl on this
# box; the local Prometheus runs in a container and reaches the box through
# the docker bridge, so BIND_ADDR=172.17.0.1 is what the dev stack needs.
# Never 0.0.0.0: these ports carry production metrics.
BIND_ADDR=${BIND_ADDR:-127.0.0.1}

if [ "${1:-}" = "--stop" ]; then
	[ -f "$PIDFILE" ] || { echo "no tunnels recorded in $PIDFILE"; exit 0; }
	while read -r pid; do kill "$pid" 2>/dev/null || true; done <"$PIDFILE"
	rm -f "$PIDFILE"
	echo "stopped"
	exit 0
fi

[ $# -eq 2 ] || {
	echo "usage: ops/dev/tunnel-prod.sh <edge-ip> <control-ip> | --stop" >&2
	exit 2
}
EDGE=$1
CONTROL=$2

ssh_opts=(-N -o ExitOnForwardFailure=yes -o ConnectTimeout=15 -o ServerAliveInterval=30)

# The edge's exporters bind its WireGuard address, not localhost, so the
# forward has to name that address rather than 127.0.0.1.
HOST_WG=${HOST_WG:-10.255.0.2}
ssh "${ssh_opts[@]}" -p "$EDGE_SSH_PORT" \
	-L "$BIND_ADDR:19100:10.255.0.1:9100" \
	-L "$BIND_ADDR:19102:10.255.0.1:9102" \
	-L "$BIND_ADDR:19200:$HOST_WG:9100" \
	-L "$BIND_ADDR:19201:$HOST_WG:9101" \
	-L "$BIND_ADDR:19202:$HOST_WG:2021" \
	"root@$EDGE" &
echo $! >"$PIDFILE"

# api-grpc publishes 9103 on the VM as 9104, on the WireGuard address only
# (ops/coolify/README.md, I-174), so the tunnel dials 10.255.255.1. The
# `api` application is supposed to publish 9103:9103; while it does not,
# fall back to the container's own address on the docker network, which is
# what an operator would reach for anyway.
api_target=$(ssh -o ConnectTimeout=15 "root@$CONTROL" '
  set -e
  if ss -lnt 2>/dev/null | grep -q ":9103 "; then echo 127.0.0.1:9103; exit 0; fi
  # Both api applications run the same image; api-grpc is the one that
  # publishes 8443, so the other one is `api`.
  for c in $(docker ps --format "{{.Names}}"); do
    docker inspect -f "{{json .Config.Entrypoint}}" "$c" | grep -q "/usr/local/bin/api" || continue
    docker port "$c" 8443 >/dev/null 2>&1 && continue
    docker inspect -f "{{range .NetworkSettings.Networks}}{{.IPAddress}}:9103 {{end}}" "$c" | awk "{print \$1}"
    exit 0
  done')

if [ -z "$api_target" ]; then
	echo "could not find the api's metrics port on $CONTROL; is the app running?" >&2
	exit 1
fi
echo "api metrics at $api_target on the control VM"

ssh "${ssh_opts[@]}" \
	-L "$BIND_ADDR:19103:$api_target" \
	-L "$BIND_ADDR:19104:10.255.255.1:9104" \
	"root@$CONTROL" &
echo $! >>"$PIDFILE"

sleep 2
for p in 19100 19102 19103 19104 19200 19201; do
	code=$(curl -sS -o /dev/null --max-time 5 -w '%{http_code}' "http://$BIND_ADDR:$p/metrics" || echo 000)
	printf '  %s:%s /metrics -> %s\n' "$BIND_ADDR" "$p" "$code"
done

echo "tunnels up (pids in $PIDFILE). Ctrl-C or --stop to close."
wait
