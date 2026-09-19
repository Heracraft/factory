# Host conventions

What every host guarantees, so hostd, infra and operators agree.

## Filesystem

| Path | What |
|---|---|
| `/var/lib/repose/hostd/` | `cert.pem`, `key.pem` (mTLS to api), `host.json` (host id, guest cidr, wg keys), `state.db` (bbolt: guest table for reconciliation) |
| `/var/lib/repose/guests/<guest_id>/` | `ch.args` (the rendered cloud-hypervisor argv, one argument per line; DECISIONS I-19), `guest.json` (non-secret copy of the guest record for `hostd reconcile --rebuild`), `ch.sock` (Cloud Hypervisor API), `vsock.sock` (host side of the guest's vsock, `CONNECT 5000` reaches guestd), `console.sock` (serial; hostd copies it into `console.log`, rotated at 64 MB keeping 3), `virtiofsd.sock`. Secrets are never written here: they are delivered to the guest's tmpfs over vsock. |
| `/var/lib/repose/builds/<revision_id>/` | `fragment.nix` for a `Build`; see `nix-build-contract.md` |
| `/var/lib/repose/base/<base_ref>/` | checkout of the platform repository at that revision (its `nix/` is the flake hostd evaluates) |
| `/run/repose/hostd.sock` | hostd's operator control socket (`hostd status`, `guests`, `snapshot-all`, `drain`, `reconcile`) |
| `/run/repose/join-token` | one-shot registration token from cloud-init, deleted after Register |
| `/nix/var/nix/gcroots/repose/<guest_id>` | GC root for the guest's system closure; removed on destroy. `rev-<project_id>-<revision_id>` roots keep the last 3 built revisions per project |
| `/dev/vg-guests/thin` | thin pool; volumes `/dev/vg-guests/g-<guest_id>` |
| `/var/log/repose/` | hostd log (journald is primary), build logs per op |

## Network

- `br-guests` bridge, host address `.1` of the host's `/22`.
- Guest address = `.2 + index` allocated by hostd from `state.db`, MAC
  derived from guest id.
- Tap per guest `tap-<8 hex of guest id>`, attached to the bridge.
- nftables table `repose`: chains `guest_fwd` (default drop; allow
  established; allow `br-guests → !br-guests` except `169.254.169.254/32`,
  `10.64.0.0/12`, and the host's own addresses; allow to the edge WireGuard IP
  on ports 8443 (hooks) and 6081 (noVNC relay)), `guest_in` (drop everything
  from guests to the host except DHCP is not used, so drop all), `nat`
  (masquerade from `br-guests`). Per-guest counters named by guest id for
  egress metering. `tc` HTB class per tap for the 200 Mbit/s shape.
- WireGuard `wg0` to the edge, host address from the edge's `10.255.0.0/16`
  pool; `AllowedIPs` on the edge side = the host's guest `/22` plus its wg
  address.
- No public IP. Outbound via Azure NAT gateway on the subnet.

## Services

`hostd.service` (Restart=always), `virtiofsd@<guest>.service` and
`guest@<guest>.service` are **transient** units created by hostd with
`systemd-run`, named so `systemctl list-units 'guest@*'` shows every guest.
`fluent-bit.service`, `node-exporter.service`, `wg-quick-wg0.service`,
`repose-snapshot.timer` (nightly 03:00 local).

## Cloud Hypervisor invocation (per guest)

hostd renders the `cloud-hypervisor` argv from the guest's system closure
(`kernel`, `initrd`, `init`, `kernel-params`) and its record (DECISIONS
I-19) and runs it with `systemd-run --unit guest@<id> --property
MemoryMax=<class RAM + 512M> --property CPUQuota=<vcpus*100>% --property
Slice=guests.slice`. The devices: `--disk path=/dev/vg-guests/g-<id>`,
`--net tap=tap-<8hex>,mac=52:54:<4 bytes of id>`, `--fs tag=ro-store,socket=
virtiofsd.sock`, `--vsock cid=<1000+index>,socket=vsock.sock`, `--serial
socket=console.sock`, `--memory size=<RAM>M,shared=on`. The CH API socket
is used for `shutdown` (after guestd's Shutdown timed out), `pause`,
`resume`, and stats.

## Operator access

Operators reach hosts over the edge's WireGuard (`ssh root@10.255.0.x` with
an operator certificate from the Host CA). Every interactive login is logged
to `audit_log` via a PAM hook that calls hostd. `repose-admin exec` is the
audited path for touching a guest.
