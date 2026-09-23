#!/usr/bin/env bash
# An 8-hour Host CA certificate for this machine's operator key, written
# next to the key so plain `ssh` offers it with no flag (DECISIONS I-177):
#
#   ops/dev/operator-cert.sh [control-ip] [key]     # defaults below
#   ssh -o HostKeyAlias=10.200.1.4 -J root@20.102.98.254:2222 root@10.255.0.2
#
# The key never leaves this machine: its public half goes to
# `repose-admin operator-cert --pubkey -` inside the api container over the
# operator SSH the control VM already accepts, and the certificate comes
# back on stdout. Hosts accept it once they know the Host CA
# (`wc -c /run/repose/host_ca.pub` non-zero; RUNBOOK "Operator certificate
# refused by a host"), which is what lets `repose.host.bootstrap
# .keyUntilHostCA` retire the plain key without locking the operator out.
# Run it again when the certificate expires; `ssh-keygen -L -f
# <key>-cert.pub` shows its validity.
set -euo pipefail

CONTROL=${1:-20.121.138.150}
KEY=${2:-$HOME/.ssh/id_ed25519}
# The `api` Coolify application's container name prefix (HANDOFF "Live
# environment"); Coolify appends a deploy suffix.
API_PREFIX=${REPOSE_API_CONTAINER_PREFIX:-8kpqxzfejbbsgwjhooep2ymc}

if [ ! -s "$KEY.pub" ]; then
	echo "no public key at $KEY.pub" >&2
	exit 1
fi

cert=$(ssh -o BatchMode=yes -o ConnectTimeout=15 "root@$CONTROL" \
	"c=\$(docker ps --filter name=$API_PREFIX --format '{{.Names}}' | head -1)
	 [ -n \"\$c\" ] || { echo 'no api container named $API_PREFIX*' >&2; exit 1; }
	 docker exec -i \"\$c\" repose-admin operator-cert --pubkey - --name \"${REPOSE_OPERATOR_NAME:-$(whoami)}\" --ttl 8h" <"$KEY.pub")

case "$cert" in
ssh-ed25519-cert-v01@openssh.com\ * | ecdsa-sha2-*-cert-v01@openssh.com\ * | ssh-rsa-cert-v01@openssh.com\ *) ;;
*)
	echo "repose-admin did not return a certificate: $cert" >&2
	exit 1
	;;
esac

umask 022
printf '%s\n' "$cert" >"$KEY-cert.pub.tmp"
mv "$KEY-cert.pub.tmp" "$KEY-cert.pub"
ssh-keygen -L -f "$KEY-cert.pub" | grep -E 'Key ID|Valid:'
