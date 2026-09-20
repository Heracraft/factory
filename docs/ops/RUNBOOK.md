# Runbook

Symptom-titled. Each alert in `../CHECKLIST.md` and `../workstreams/10-observability.md`
has an entry with the same name. Commands assume an operator certificate
(`repose-admin operator-cert`) and the edge WireGuard up on the operator's
machine.

## First-time setup

### Operator machine

```bash
nix develop                                  # go, tofu, az, wg, promtool, grafana-cli
az login
export DATABASE_URL=...                      # repose-admin talks to Postgres directly (I-42); over the Coolify VM's WireGuard address
repose-admin operator-cert                  # 8h Host-CA cert, add it next to ~/.ssh/id_ed25519
sudo wg-quick up ops/wg/operator.conf        # 10.255.0.0/16 reachable
```

### Control plane (Coolify VM)

The full click path, with the reasons, is `coolify.md`. The short form:

1. `make -C infra apply ENV=prod` with `coolify_count = 1` in `prod.tfvars`.
   The apply does not return until Coolify's container is healthy. Coolify's
   dashboard is on **8000, plain HTTP, and deliberately not in the NSG**, so
   reach it through the tunnel and create the admin user:
   `ssh -N -L 8000:127.0.0.1:8000 root@<control ip>`, then
   `http://127.0.0.1:8000`. Add the server as `localhost`.
2. Copy `/data/coolify/source/.env` into the password manager **now**. Its
   `APP_KEY` is what decrypts every credential in the Postgres dump; without
   it the dump restores a database of ciphertext.
3. Add Logto as a Docker Image resource (`ghcr.io/logto-io/logto`), Postgres
   as a Coolify database, run its migration, set `ENDPOINT` and
   `ADMIN_ENDPOINT` to `auth.repose.herakraft.co`. Configure the GitHub
   connector and two applications: `repose-cli` (Native, device flow on)
   and `repose-web` (SPA). Create API resource
   `https://api.repose.herakraft.co`.
4. Add `api` and `web` as Dockerfile applications from the repo, health
   checks `/healthz` and `/`, env from `ops/coolify/api.env.example`.
   Secrets (CA keys, Key Vault client cert, Stripe keys, Resend key) as
   Coolify secrets. A health check on every app is not optional: without one
   Coolify silently falls back to stop-then-start instead of a rolling deploy.
5. Add the platform Postgres as a Coolify database with S3 backup to R2,
   nightly 02:00, retention 35 days. The form's fields are
   `tofu -chdir=infra/r2 output coolify_s3_destination`. Run
   `repose-admin db migrate`, then one manual backup, then
   `ssh root@<control ip> repose-backup-check`.
6. Add the Coolify VM as a WireGuard peer of the edge (`ops/wg/coolify.conf`).

### Edge

`make -C infra apply ENV=prod`; nixos-anywhere installs `.#edge`. Then
`repose-admin edge init` writes the WireGuard hub key into the api and the
gateway's mTLS client cert. Verify `ssh -p 22 probe.nobody@ssh.repose.herakraft.co`
returns `certificate required`.

### First host

See `../workstreams/11-infra-opentofu.md` §5 "Adding a host". Then
`repose-admin hosts list` shows `ready`, and `repose-admin hosts smoke
<host>` creates, starts, snapshots and destroys a throwaway guest.

### Observability

On the personal server, from this repository's `ops/` (its README has the
copy-paste version):

- `ops/prometheus/prometheus.yml` is the scrape config; hosts are listed in
  `ops/prometheus/targets/hosts.yml`, which Prometheus re-reads every minute,
  so adding a host needs no restart. Run Prometheus with
  `--storage.tsdb.retention.time=90d`.
- `ops/alerts.yaml` goes in its rule files, `ops/alertmanager/repose-route.yaml`
  into Alertmanager (ntfy to the owner).
- `ops/grafana/provisioning/` and `ops/dashboards/*.json` are Grafana's
  provisioning; the seven dashboards appear in a `repose` folder.
- `ops/loki/retention.yaml` sets 90 days for component logs and 30 for guest
  console logs, and needs the compactor enabled to do anything.
- The server joins the edge's WireGuard as one more peer:
  `ops/prometheus/wireguard-peer.conf`. Nothing is scraped over the internet.
- Locally, `docker compose -f ops/dev/docker-compose.yml up -d` is the same
  Grafana with the same dashboards and no data.

## Common operations

| Task | Command |
|---|---|
| List hosts with capacity | `repose-admin hosts list` |
| Add a host | `hostdev init --host host-NN` (M1) or `repose-admin hosts add --name host-NN` (once the api exists), then `make -C infra apply ENV=prod` (workstream 11) |
| Drain a host (no new placements) | `repose-admin hosts drain host-NN` |
| Retire a host (after all projects moved) | `repose-admin hosts retire host-NN` |
| Move a project to another host | `repose-admin projects move <id> --to host-NN` (stop, snapshot, restore, start) |
| Suspend a user | `repose-admin users suspend <handle> --reason "..."` (stops guests, freezes billing, audit row) |
| Unsuspend | `repose-admin users unsuspend <handle>` |
| Run a command in a guest (audited) | `repose-admin exec <project-id> -- <argv>` |
| Force a snapshot | `repose-admin projects snapshot <id>` |
| Restore a snapshot | `repose-admin projects restore <id> --snapshot <sid> [--to host-NN]` |
| Revoke all certs for a user | `repose-admin certs revoke --user <handle>` |
| Rotate a host's mTLS cert | `repose-admin hosts rotate-cert host-NN` |
| Rotate the Key Vault wrapping key | `az keyvault key rotate` then `repose-admin secrets rewrap` |
| Query audit log | `repose-admin audit --user <handle> --since 24h` |
| Publish a base version | `repose-admin base publish --rev <git sha> --changelog "..." [--security]` |
| Smoke-test a host | `repose-admin hosts smoke host-NN` (create, snapshot, stop, start, destroy a throwaway guest) |
| Initialise the CAs (once) | `repose-admin ca init`; then `repose-admin ca sign-client --name gateway --out <dir>` for the edge |
| Record the edge WireGuard hub | `repose-admin edge init --endpoint <ip>:51820 --pubkey <wg pub> [--out <dir>]` |
| Re-run the hourly rollup | `repose-admin billing rollup --hour 2026-09-17T14` |
| Database migration | `repose-admin db migrate` / `repose-admin db rollback --to NNNN` |

## HostMemory80

Reserved memory on a host is above 80 percent.

1. `repose-admin hosts list`: confirm which host and how many guests are
   `stopped` (stopped guests hold no memory reservation; if the number is
   wrong, the api's reservation accounting drifted, see "Reservation
   drift").
2. Add a host (workstream 11 §5). Until it is `ready`, the scheduler still
   places on the full host; drain it if placements must stop now.

## HostUnreachable

No heartbeat for 90 seconds.

1. From the operator machine: `ping 10.255.0.<host>` over WireGuard. If
   that fails too, check the Azure portal for the VM state; a platform
   repair event shows there.
2. If the VM is up: `ssh root@10.255.0.<host>`, `systemctl status hostd`,
   `journalctl -u hostd -n 200`. A `stream_disconnect` loop with TLS errors
   means the host cert expired: `repose-admin hosts rotate-cert` from the
   api side writes a new one via the edge jump.
3. If the VM is gone: "Host loss" below.
4. Guests keep running through an api outage; the only thing lost is
   metering samples for the window, which the rollup marks as `gap` rather
   than zero.

## SnapshotStale

A running project's newest snapshot is older than 36 hours.

1. `repose-admin projects show <id>`: last snapshot, last error.
2. `journalctl -u hostd | grep snapshot_fail` on the host. Common causes:
   `freeze_timeout` (guestd hung; see "Guest unresponsive"), Blob auth
   (managed identity lost its role: `az role assignment list`), pool out of
   metadata space (`lvs -a`; extend the pool metadata).
3. `repose-admin projects snapshot <id>` after fixing; confirm the age
   gauge drops.

## BuildQueueStuck

Two builds running on a host with no completion for 45 minutes.

1. `repose-admin ops list --host host-NN --state running`.
2. On the host: `ps aux | grep nix-build`; `journalctl -u hostd | grep
   build_`. A build past its 30-minute cap should have been killed by
   hostd; if it is still running, hostd's limiter failed: `kill` it,
   restart hostd, open an issue.
3. If both builds are legitimately slow (big closures, cold cache), wait;
   the alert clears on completion. Consider the central builder (deferred).

## GatewayAuthSpike

More than one auth failure per second at the gateway.

1. Grafana gateway dashboard: failures by `reason`. `no_cert` or `bad_ca`
   in volume from one source is a scan; the gateway rate-limits per source
   IP after 20 failures (fail2ban-style, built in) and refuses further auth
   for 10 minutes. Nothing to do unless it persists for hours; then add the
   source to the edge NSG deny list. `route_error` in volume means the
   gateway cannot reach the api (see "Gateway relay failures").
2. `expired` in volume means the CLI's silent refresh is broken for many
   users: check `repose_api_certs_issued_total` fell off a cliff, and the
   Logto token endpoint.
3. `wrong_principal` from one user repeatedly is someone probing other
   projects; `repose-admin audit --user`.

## Gateway relay failures

The failure modes of the SSH gateway (docs/workstreams/06-gateway-edge.md
§6), and the message the user sees for each. The gateway is on the edge;
reach it with `ssh -p <edge_operator_ssh_port> root@<edge ip>` and read
`journalctl -u gateway` (events are structured JSON: `auth_fail`,
`route_fail`, `dial_fail`, `session_open`, `session_close`).

| Symptom / user message | Cause | What to do |
|---|---|---|
| `gateway cannot reach control plane; try again shortly` | the api is down or the CA/revocation caches aged past 1 h | `journalctl -u gateway \| grep route_fail`; check the api and `api-grpc` app; existing sessions keep working, new ones resume when a refresh succeeds |
| `environment is not accepting connections yet` | the guest's sshd is not up yet, or dialed a throwaway host key | the CLI retries 60 s after a `start`; if it persists, `repose-admin projects show` for the guest state, then "Guest not ready" |
| `cannot reach environment: no route to host` | no WireGuard peer or route for the guest's host | on the edge `wg show wg0` and `ip route \| grep <guest cidr>`; `wgsync` adds them from `/internal/hosts` within 30 s — see "HostWgDown" |
| `permission denied (certificate expired)` / `... not yet valid` | the user's certificate is outside its 12 h validity | the CLI refreshes and retries once; a spike of `expired` is "GatewayAuthSpike" step 2 |
| `permission denied (certificate revoked)` | logout or a revoked serial | expected; takes effect within 30 s of `repose logout` |
| `certificate not valid for this project` | the certificate's principals do not contain the resolved project id | the CLI re-requests a cert for the project; repeated from one user is probing ("GatewayAuthSpike" step 3) |
| `<slug> is stopped; run \`repose start\`` | the project is stopped | expected; the user starts it |
| `gateway busy` | the 200-connection cap is reached | alert on `repose_gateway_sessions`; if legitimate, the edge is undersized |
| `too many authentication attempts from your address; try again later` | 4 concurrent auths or 20 failures from one source | a scan; the ban clears in 10 min |
| `login name must be <project>.<user>` | a malformed SSH login name | the user's SSH config is wrong; `repose run` rewrites it |

A gateway restart (a deploy) drops every relay; the guest's tmux session
survives, so `repose attach` reconnects. `nixos-rebuild switch --rollback`
on the edge restores the previous gateway in seconds, and `wgsync` rebuilds
the peer set within 30 s.

## EgressHigh

A project moved more than 1 TB in 24 hours.

1. Abuse dashboard: the project's `proc_samples` top `comm`. A torrent
   client, a scraper, or a miner's pool traffic looks different from
   `docker pull`.
2. If legitimate, nothing (it is metered and billed). If not:
   `repose-admin users suspend`.

## PoolFull

Thin pool under 10 percent free.

1. `lvs vg-guests`: data percent and metadata percent. Metadata full is
   worse (writes fail across every volume): `lvextend --poolmetadatasize`.
2. Extend the data disk in Azure (`tofu apply` with a larger
   `data_disk_gb`, online for Premium SSD v2), then `pvresize` and
   `lvextend -l +100%FREE vg-guests/thin`.
3. Find who: `repose-admin projects list --host host-NN --sort disk`.
   Over-allocated thin volumes are fine; used space is what matters.

## StoreFull

Host root filesystem over 85 percent, almost always `/nix/store`.

1. `nix path-info -S --all | sort -k2 -n | tail`: biggest closures.
   hostd keeps a GC root per live guest closure; everything else is
   collectable.
2. `nix-collect-garbage` (hostd runs it weekly; run it now). If still
   full, a tenant's fragment pulled something huge: `repose-admin
   projects list --host host-NN --sort closure` and talk to them, or
   lower the closure cap.

## GuestdLost

hostd cannot talk to a guest's guestd for 5 minutes.

1. `repose-admin projects show <id>`: is the guest `running`? If the
   guest crashed, the transient unit is gone: `repose-admin projects
   start <id>`.
2. If running: the guest's console log at
   `/var/lib/repose/guests/<id>/console.log`. An OOM in the guest (the
   tenant filled memory) usually killed guestd; the kernel line names it.
   `repose-admin projects restart <id>` (stop without snapshot, since
   freeze needs guestd, then start).
3. Sampling for that guest is missing for the window; billing uses the
   last known state, so a running guest is still billed.

## HostScrapeDown

Prometheus cannot scrape a host: `up{job=~"hosts|hostd"} == 0` for 5 minutes,
and that host's panels on Host capacity go blank. Metering is *not* affected:
hostd sends samples to the api over its own gRPC stream, so billing data
keeps arriving (docs/workstreams/10-observability.md §6).

1. From the monitoring server: `curl -s http://<host wg addr>:9101/metrics |
   head -1`. A timeout is the tunnel, a connection refused is hostd.
2. Tunnel: "HostWgDown" above. The scrape and the logs use the same path, so
   a FluentBitStuck alert for the same host confirms it.
3. hostd itself: on the host, `systemctl status hostd` and
   `ss -tlnp | grep 9101`. hostd binds the WireGuard address, so a hostd that
   started before wg0 existed still listens (`ip_nonlocal_bind`); if it does
   not, `systemctl restart hostd`.
4. nftables: `nft list chain inet repose input` must admit 9100, 9101 and the
   Fluent Bit metrics port from `wg0`.
5. Nothing to do about the gap: Prometheus has no backfill. Say so in the
   incident note rather than wondering later why a graph has a hole.

## FluentBitStuck

A host's Fluent Bit has been failing to ship to Loki for 30 minutes
(`increase(fluentbit_output_retries_failed_total[30m]) > 0`). It buffers to
disk and retries forever, so nothing is lost yet; at 1 GB the oldest chunks
are dropped.

1. Is Loki up? `curl -s http://<loki>:3100/ready` from the monitoring server.
   If Loki is the problem, every host alerts at once.
2. On the host: `systemctl status fluent-bit`, `journalctl -u fluent-bit -n
   50`. `ConditionPathExists=/run/repose/host.env` unmet means the host never
   registered ("HostUnregistered"); the unit is `partOf`
   `repose-host-net.service`, so `systemctl restart repose-host-net` restarts
   it with freshly rendered addresses.
3. Buffer size: `du -sh /var/lib/fluent-bit/storage`. Approaching 1 GB is the
   deadline for fixing Loki before lines are dropped.
4. Wrong Loki address: `grep LOKI /run/repose/host.env`. It comes from
   `loki_url` in `host.json`, which the api sends at registration; correct it
   there and `systemctl restart repose-host-net`.
5. Guests are unaffected throughout: nothing in a guest waits on log
   shipping.

## Guestd not ready

A guest is `starting` and never reaches `running`, or the api shows
`guestd_ok=false` from the first sample. guestd sends `Ready` only once sshd
is listening, so "not ready" means guestd did not start, or sshd did not.

1. Console log at `/var/lib/repose/guests/<id>/console.log`. The line to look
   for is guestd's own: `{"event":"ready","msg":"listening",...}` with
   `"transport":"vsock"`. If it is absent, guestd did not start; the lines
   above it say why (a missing `/run/repose`, a vsock device the runner did
   not attach).
2. If guestd is listening but no `Ready` followed, sshd is the one that did
   not come up: the same console log has sshd's error. The usual cause is
   sshd material that never arrived, so `/run/repose/ssh_host_ed25519_key` is
   missing; `repose-admin projects restart <id>` re-sends `CreateGuest`'s
   secrets.
3. From the host, talk to the guest directly:
   `repose-admin exec <id> -- guestd call ping --cid <vsock cid>`. A response
   means guestd is fine and the problem is on hostd's side of the vsock; no
   response with guestd listening means the CID is wrong in hostd's state.
4. The guest keeps running through all of this. A tenant with a certificate
   can still SSH in; what is lost is sampling, secrets delivery and config
   apply.

## Freeze timeout

A snapshot failed with `freeze_timeout`, or the alert fired from a
`host_warning` of that kind.

1. This is guestd's watchdog doing its job: it froze the root filesystem for a
   snapshot, no `Thaw` arrived within 10 seconds, and it thawed itself. The
   guest is *not* wedged; nothing needs to be unfrozen by hand.
2. The cause is on the host side: hostd died mid-snapshot, or the LVM
   snapshot took longer than the window. `journalctl -u hostd | grep
   snapshot` on the host gives which.
3. If the thin pool is near full, the LVM snapshot is what was slow: see
   "PoolFull" above, then `repose-admin projects snapshot <id>` again.
4. If it repeats for one project only, the guest's root filesystem has a
   writer that will not quiesce (a database in a container). Stop the guest
   and snapshot from stopped: `repose-admin projects restart <id>` takes the
   snapshot on the way through.
5. To confirm the guest is healthy afterwards:
   `repose-admin exec <id> -- guestd call ping` and check `df` inside.

## Switch failed

`repose config apply` reported a failed revision, or a base bump left a
project on the old system.

1. The Nix output is stored with the revision: `repose-admin ops list
   --project <id>` then `ops log <op id>`. The tail of it is the reason; it
   is `switch-to-configuration`'s own output, verbatim.
2. The old system is still active and the guest is still running. This is
   switch-to-configuration's semantics and guestd relies on it: a failed
   switch changes nothing.
3. The common causes, in order: a systemd unit in the user's fragment that
   fails to start (the output names it), a store path missing from the share
   (guestd also sends `store_path_missing`; see "StoreFull" for why a path
   disappears, then `repose-admin projects restart <id>` to rebuild and
   re-register the GC root), and a closure that needs a reboot
   (`needs_reboot=true` is not a failure: the user is told to run `repose
   config apply --reboot` when their agent is idle).
4. To retry by hand once the fragment is fixed: `repose config apply` again.
   Nothing needs cleaning up first.

## RollupLag

The hourly usage rollup is more than 2 hours behind.

1. `journalctl` on the api container for `rollup_` events. A failing hour
   is retried; a poisoned row (a sample with a negative delta from a
   hostd restart) is skipped and logged with the project id.
2. `repose-admin billing rollup --hour <hour>` to re-run one hour.

## PartitionDropFail

The hourly `repose_partitions_maintain()` is failing: `meter_samples` and
`proc_samples` keep partitions past their 90 and 30 day retention, so
Postgres grows. Nothing else breaks and no data is lost
(docs/workstreams/10-observability.md §6).

1. The api's log says why: `{component="api"} | json | event="partition_drop_fail"`.
2. By hand, as the api's role: `select * from repose_partitions_maintain();`
   It prints one row per create and drop. A permission error means the role
   cannot `drop table`; a lock timeout means something is reading a partition
   it wants to drop, and the next hour will get it.
3. Space now, if that is the pressure:
   `select relname, pg_size_pretty(pg_total_relation_size(oid)) from pg_class
   where relname like 'proc_samples_%' order by relname;` then
   `drop table proc_samples_YYYYMM` for a month wholly past retention.
4. If the *create* half failed, inserts for the new month will fail at 00:00
   on the first: `select repose_partition_create('meter_samples',
   date_trunc('month', now())::date);` is the fix, and hostd's sample buffer
   holds what did not land (workstream 03).

## StripePushFail

Usage records failed to push.

1. Stripe dashboard, API logs. A 400 usually means the subscription item
   id changed (a user changed plan): `repose-admin billing resync
   --user`.
2. Rows keep `stripe_usage_record_id = null` and are retried hourly; no
   usage is lost.

## HostUnregistered (host never registered)

A new host has been up for more than five minutes and is not in `hosts
list`.

1. `ssh -J root@<edge ip>:2222 root@<private ip>`: `journalctl -u hostd`.
   `token_expired` or `token_used`: mint a new one (`hostdev init --host
   <name> --reissue` for M1, `repose-admin hosts add --reissue` once the api
   exists), put it in `infra/azure/prod/prod.local.tfvars` and
   `make -C infra apply ENV=prod` (only the token-delivery step re-runs), or
   by hand `install -d -m 0700 /run/repose && umask 077 && cat >
   /run/repose/join-token` and `systemctl restart hostd`. The token is never
   passed as a command-line argument, so it does not land in a shell history
   or an apply log.
2. `kvm_missing`: the VM was created without `security_type = Standard`.
   Destroy and recreate; there is no in-place fix. The apply should not have
   got this far: the host module's post-install check fails when `/dev/kvm`
   is missing, and the tfsec rule REPOSE-VM-001 fails the build when Secure
   Boot is set.
3. `pool_missing`: the data disk is not attached or `disko` did not run.
   `lsblk` and `ls -l /dev/disk/azure/scsi1/lun10`; re-run nixos-anywhere if
   the layout is missing.
4. Nothing in the journal at all and SSH refused: the install may have
   stopped mid-kexec. `az vm boot-diagnostics get-boot-log --name <host> -g
   repose-prod` shows the serial console; `make -C infra apply ENV=prod`
   after `tofu -chdir=infra/azure/prod taint
   'module.environment.module.host["<host>"].module.install.terraform_data.install'`
   retries the install from the image. The data disk is a separate resource
   with `prevent_destroy`, so it is not touched.

## PoolHigh (thin pool at 80 percent)

`host_warning{kind="pool_high"}` from hostd, or
`repose_lvm_pool_data_percent > 80` from the host's textfile collector.
lvm.conf autoextends the pool by 10 percent of its size into the 5 percent
VG headroom at this threshold, once; after that the pool fills for real.

1. `lvs vg-guests` on the host: data and metadata percent, and whether
   `thin` still has room to grow (`vgs vg-guests` free space). Metadata
   over 80 percent is worse; see "PoolFull".
2. Plan the disk grow now: `tofu apply` with a larger `data_disk_gb`
   (online for Premium SSD v2), then on the host `pvresize <pv>` and
   `lvextend -l +95%FREE vg-guests/thin`. At 90 percent hostd refuses
   `CreateGuest` and `Restore` with `insufficient_capacity`; existing
   guests keep running.
3. `repose-admin projects list --host host-NN --sort disk` for who is
   using it; `repose-admin hosts drain host-NN` if the grow cannot happen
   before it fills.

## StoreHigh (store at 80 percent)

`host_warning{kind="store_high"}` from hostd; the root filesystem is over
80 percent, almost always `/nix/store`. `nix.settings.min-free` (50 GB)
already triggers GC of unrooted paths during builds and hostd refuses
`Build` with `insufficient_capacity: host store full` at this point.

1. `nix-collect-garbage` on the host now (the weekly timer keeps 14 days).
   hostd's GC roots under `/nix/var/nix/gcroots/repose/` protect every
   guest's closure and the last three revisions per project; everything
   else goes.
2. Still high: `nix path-info -S --all | sort -k2 -n | tail` for the
   biggest closures and `repose-admin projects list --host host-NN --sort
   closure`; a tenant near the 20 GB closure cap on a small host is the
   usual cause. See "StoreFull" for the 85 percent alert.

## Reaching a guest before WireGuard exists (M1)

Until workstream 06 lands, hosts are not on WireGuard and the gateway does
not exist. Operators reach a guest by jumping through the edge and the host:

1. `ssh -J root@<edge ip> root@<host private ip>` (bootstrap sshd on the
   provider NIC, `repose.host.bootstrap.enable`, DECISIONS I-40).
2. Nothing to do: `guest_in` admits replies to flows the host itself
   opened (`ct direction reply ct state established,related`, DECISIONS
   I-74), so the host reaches a guest's sshd and a reload does not undo it.
   Guests still cannot open anything towards the host; on a host built
   before I-74 the equivalent is the runtime
   `nft insert rule inet repose input iifname "br-guests" ct state established,related accept`,
   removed again with `nft -a list chain inet repose input` and
   `nft delete rule inet repose input handle <n>`.
3. `hostdev ssh-cert --project <p> --pubkey ~/.ssh/id_ed25519.pub >
   ~/.ssh/id_ed25519-cert.pub` on the edge, then
   `ssh -J root@<edge ip>,root@<host ip> dev@<guest ip>`.

## Host never configured its bridge (registration ran, br-guests has no address)

`hostd status` says registered and `stream_connected`, but `ip addr show
br-guests` has no `10.64.x.1` address, `/run/repose/host.env` is missing,
and node_exporter or Fluent Bit are inactive. `repose-host-net` renders
those from `host.json` and is restarted by `repose-register.service`'s
ExecStartPost; when the unit failed for any reason other than the token
(seen 2026-09-20: it exited 2 on an unknown flag, DECISIONS I-40) hostd
registered by itself and nothing restarted the renderer.

1. `journalctl -u repose-register -u repose-host-net` for the cause.
2. `systemctl restart repose-host-net.service`; then `ip -br addr show
   br-guests` shows the `.1/22` address and `cat /run/repose/host.env` has
   `HOST_ID` and `GUEST_CIDR`.
3. hostd restarts `repose-host-net` itself after a self-registration since
   I-40, so on a current host this entry means the renderer itself failed.

## Store writes fail with `Read-only file system` on `/nix/store/.links`

`nix copy` to the host, hostd's `Build`, or `nix-store --optimise` fail with
`creating hard link ... /nix/store/.links/...: Read-only file system`, while
`/nix/store` itself is the usual read-only bind. `findmnt /nix/store/.links`
shows the `repose-links-mask` tmpfs: the mask `repose-store-export.service`
puts over `/run/repose/store-export/.links` propagated back to the store
because the bind shared `/`'s peer group (DECISIONS I-61, fixed by making
the export mount private). On a host built before the fix:
`umount /nix/store/.links`, and confirm
`ls -A /run/repose/store-export/.links` is still empty.

## HostWgDown

The edge cannot reach a host's guests: `wg show` on the edge shows no
recent handshake for the host's peer, `dial_fail` in gateway logs for its
guests, alert `host_wg_down`. hostd's gRPC stream goes over the provider
NIC, so the api still sees the host as `ready`.

1. On the host (via the Azure serial console or the bootstrap key if the
   tunnel is the only way in): `systemctl status wg-quick-wg0`,
   `wg show wg0`. `ConditionPathExists=/run/repose/wg0.conf` unmet means
   the host never registered: "HostUnregistered".
2. `journalctl -u repose-host-net`: the render from `host.json` failed
   (malformed `host.json` after a bad rotation) or the endpoint did not
   resolve at boot. `systemctl restart repose-host-net` re-renders and
   restarts wg0, sshd, node_exporter and Fluent Bit.
3. Keys do not match: `repose-admin hosts rotate-wg host-NN` issues a new
   pair, hostd rewrites `host.json` and restarts `repose-host-net`;
   confirm the edge's `wgsync` picked up the new public key within 30 s.
4. Handshake fine but no route: on the edge `ip route | grep <guest cidr>`;
   `wgsync` adds it from `/internal/hosts`.

## Host rebooted

`Hello` after boot reports every guest `stopped`; the api restarts those
that were `running` and notifies their users (workstream 03). Kernel
panics are not auto-rebooted (`panic=` is unset on purpose); a stuck host
is restarted from the Azure portal.

1. `journalctl -b -1 -p err` on the host for why. An OOM in the host
   itself means the `guests.slice` cap was wrong for the RAM: check
   `systemctl show guests.slice -p MemoryMax` against `free -b`.
2. Check that everything came back: `systemctl --failed`, `nft list table
   inet repose`, `ip addr show br-guests`, `wg show wg0`, `lvs vg-guests`,
   `systemctl list-units 'guest@*'`.
3. If the reboot was for a kernel update it should have been a drain
   (`repose-admin hosts drain`, `nixos-rebuild boot`, reboot, undrain);
   an unplanned reboot with tenants on the host is an incident line in
   `docs/incidents/`.

## Guest unresponsive

A tenant reports `repose attach` hangs, or `Freeze` times out.

1. `repose-admin projects show`: state and last signals. `guestd_ok=false`
   means "GuestdLost" above.
2. `repose-admin exec <id> -- uptime` (goes through vsock; if it works,
   the guest is fine and the problem is the gateway or the tenant's
   certificate). From the host itself, `guestd call ping --cid <vsock cid>`
   asks guestd directly, without the api in the path.
3. Console log for kernel panics or OOM. A panic leaves CH running with a
   dead guest: `repose-admin projects restart`.

## Guest not ready

hostd reports `guest did not become ready` (no `Ready` from guestd within
120 s of start), or the api shows a project `starting` for minutes.

1. Console log: `/var/lib/repose/guests/<id>/console.log` on the host.
   The last lines say where boot stopped. A kernel panic or `init=` not
   found means the runner's closure is gone from the host store ("Store
   mount missing" below covers the guest side; on the host, `ls -l
   /nix/var/nix/gcroots/repose/<id>`).
2. `systemctl status guest@<id> virtiofsd@<id>` on the host. If virtiofsd
   is not running, the guest is stuck in the initrd waiting for the
   `ro-store` tag: start it and restart the guest. `guest@<id>` runs as
   the `hostd` user (I-51): `Permission denied` on `/dev/kvm`, the tap or
   `/dev/vg-guests/g-<id>` in `journalctl -u guest@<id>` means the host
   lost `hostd`'s `kvm` membership, the tap's owner, or the udev rule
   that makes `g-*` volumes group `hostd` (`ls -l /dev/mapper/vg--guests-g--*`
   should say `root hostd`); a create that fails at step 5 naming a user
   means the `hostd` or `virtiofsd` account is missing.
3. If boot completed (`multi-user.target` in the console) but no `Ready`:
   guestd crashed. The console carries guestd's own stderr (it logs to the
   console so a frozen root never blocks it); `repose-admin exec <id> --
   systemctl status guestd` works only once it is up, so read the console.
   A crash loop in guestd is a base bug: `hold_base_updates` the project,
   roll the base back (`repose-admin base rollback`), open an issue.
4. sshd refusing the certificate after `Ready` is a principals or CA
   problem, not readiness: `repose-admin exec <id> -- cat
   /etc/ssh/principals/dev /etc/ssh/user_ca.pub` must show the project id
   and the User CA; `SetPrincipals` and `UpdateSecrets` from the api
   rewrite them and reload sshd.

## Store mount missing

A guest logs `mount: /nix/.ro-store: wrong fs type` or `virtiofs: tag
ro-store not found` in its console, commands fail with `No such file or
directory` for store paths, or guestd sends `Warning{store_path_missing}`.

1. On the host: `systemctl status virtiofsd@<id>`; the unit must be running
   on `/var/lib/repose/guests/<id>/virtiofsd.sock` with `--shared-dir
   /nix/store`. If it exited, `journalctl -u virtiofsd@<id>` says why
   (usually the socket directory or the `virtiofsd` user's permissions).
   Restart it, then `repose-admin projects restart <id>`; a guest cannot
   re-mount the share on its own.
2. `store_path_missing` with virtiofsd healthy means the host garbage
   collected a path the guest's system uses: the GC root under
   `/nix/var/nix/gcroots/repose/` is gone. `nix build` the guest's
   revision again on the host (deterministic), re-root, restart the guest.
   That is a hostd bug; record it.
3. Paths a user installed in the guest are never affected: they live in
   the guest's overlay upper dir (`/nix/.rw-store`), and
   `repose-pin-profile` copies shared paths of the profile there whenever
   the profile changes (`journalctl -u repose-pin-profile` in the guest
   shows `pinned N store paths`).

## Docker driver wrong

`docker info` in a guest shows a storage driver other than `overlay2`, or
`docker run` fails with `overlay2 not supported`.

1. `mount | grep ' / '` in the guest must show ext4 on `/dev/vda`. Anything
   else means the thin volume was created without `mkfs.ext4` or the
   runner booted the wrong disk: check `lvs vg-guests` and the guest's
   `bin/run --volume` argument in `systemctl cat guest@<id>`.
2. `lsmod | grep overlay` must list the module; the base loads it in the
   initrd. A base whose kernel dropped it is caught by the `guest-docker`
   VM test before publish; if a published base has it, `repose-admin base
   rollback` and hold the affected projects.
3. `/var/lib/docker` full: the guest's volume is at capacity; `repose
   resize` (the user) or `repose-admin projects resize` (operator).

## Desktop not starting

`repose open --desktop` hangs, or the browser shows a connection error on
6080.

1. In the guest: `systemctl status repose-novnc.socket repose-xvfb
   repose-x11vnc repose-novnc`. The socket must be `listening`; a
   connection to 127.0.0.1:6080 starts the proxy, which requires the whole
   chain. `journalctl -u repose-x11vnc` failing at `ExecStartPre` means the
   password step could not write `/run/repose/desktop` (must be 0700 dev;
   tmpfiles recreates it at boot).
2. `Xvfb` failing with `Cannot establish any listening sockets` means a
   stale `/tmp/.X11-unix/X99` lock from a killed server: remove
   `/tmp/.X99-lock` and `/tmp/.X11-unix/X99`, then reconnect.
3. The chain stopped by itself: that is the 30-minute idle stop
   (`journalctl -u repose-desktop-idle`); reconnecting starts it again with
   a new password (`repose-guest-profile desktop start` prints it).
4. A headed browser shows nothing on the desktop: the shell that launched
   it had no `DISPLAY` because it started before Xvfb. New shells export
   `DISPLAY=:99` while the X socket exists; open a new tmux window.

## Reservation drift

`hosts list` free memory does not match the sum of running guests.

`repose-admin hosts reconcile host-NN` asks hostd for its `Hello` state
and recomputes reservations. Happens after an api crash mid-command; the
reconcile is safe to run any time.

## Coolify deploy failed

The api or web app's rolling deploy did not go green.

1. Coolify's deployment log. A health check timeout with the container
   alive is usually a migration running long (the api runs migrations at
   start): wait, the old container is still serving.
2. A migration failure leaves the new container crash-looping and the old
   one serving. Fix forward or `repose-admin db rollback --to <previous>`
   from the operator machine (it connects to Postgres over the Coolify
   VM's WireGuard address), then redeploy the previous image tag.
3. If the deploy fell back to stop-then-start (Coolify does this silently
   when the health check is missing), the health check config was lost;
   restore it before the next deploy.

## PostgresBackupStale

No Postgres dump has landed in R2 for more than 36 hours. Coolify reports
backup failures only in its own UI, which nobody is watching at 02:00, so the
check is a command:

```bash
ssh root@<control ip> repose-backup-check
```

It exits 0 with the age of the newest object, 1 when that is over 36 hours or
the bucket is empty, and 2 when the machine has no rclone remote named `r2`
(the fix is in the script's own header, and in the `rclone_hint` field of
`tofu -chdir=infra/r2 output coolify_s3_destination`).

1. Exit 2 means the check was never wired up, not that the backup failed. The
   remote is created once from the R2 API token.
2. Otherwise Coolify's backup job log for the Postgres resource. An
   authentication error is usually the endpoint without its scheme or the
   region left blank instead of `auto` (`coolify.md`).
3. A disk-full on the control plane fails the dump before the upload: the
   dump is written locally first. `df -h /data` on the VM.
4. Run a manual backup from the UI once the cause is fixed, and re-run
   `repose-backup-check`.

The bucket's 35-day lifecycle rule keeps deleting old dumps while this alert
is open, so a week of failures is a week closer to having no restore point at
all.

## Coolify dashboard unreachable

`http://<control ip>:8000` times out. That is correct: the control subnet NSG
does not open 8000, and `infra/azure/modules/network/main.tf` has a
postcondition that fails the plan if somebody adds it. Coolify's dashboard is
plain HTTP and unauthenticated until an admin account exists.

```bash
ssh -N -L 8000:127.0.0.1:8000 root@<control ip>   # then http://127.0.0.1:8000
```

If the tunnel connects but nothing answers, the installer did not finish:
`docker logs coolify` and `/var/log/cloud-init-output.log` on the VM. A fresh
apply would have failed at `terraform_data.ready` rather than returning, so
this is a machine that was healthy and stopped being one.

## ssh.repose.herakraft.co resolves to Cloudflare

`dig +short ssh.repose.herakraft.co` returns `104.21.x.x` or `172.67.x.x`
instead of the edge's address, and SSH hangs or is refused. `herakraft.co`
answers every name under it from a **proxied wildcard record**, so a missing
`ssh.repose` record does not fail, it resolves to Cloudflare's proxy, which
carries neither SSH nor WireGuard. Every host configured with that hostname as
its WireGuard endpoint fails the same way.

1. `make -C infra plan ENV=prod` says so too, as the `dns_is_managed_or_manual`
   check warning, whenever `manage_dns` is false.
2. Fix: create `ssh.repose` as an **unproxied** A record pointing at the
   `edge_public_ip` output, or set `manage_dns = true` with a
   `CLOUDFLARE_API_TOKEN` and let OpenTofu own it. `infra/README.md`,
   "DNS while manage_dns is false", has the full record table.
3. Until then the edge is reachable at its literal address, which is what
   every `ssh_jump` output already prints.

## Postgres restore

The dump in R2 is only half of a restore. Coolify encrypts the credentials it
holds with `APP_KEY` from `/data/coolify/source/.env`; a dump restored without
that file is a database of ciphertext. Start from both.

1. Provision a scratch Coolify (or use staging), add a Postgres database,
   download the newest dump from R2 (`rclone ls r2:repose-pg-backups`),
   `pg_restore` into it. `rclone` and `pg_restore` are installed on the
   control-plane VM by cloud-init, and the apply fails if they are missing.
2. Point a staging api at it, run `repose-admin db verify` (row counts
   per table against the last rollup), time the whole thing, record it in
   `../CHECKLIST.md`'s release item.
3. For a real restore: stop the api (Coolify stop), restore into the prod
   database, start the api. Hosts reconnect and `Hello` reconciles guest
   state; anything created between the dump and the failure is
   re-registered by hostd's state and shows as `orphan` in `hosts
   reconcile`, which offers to adopt or destroy each.

## Host loss

A host is gone (Azure repair, disk lost, or a deliberate retirement without
drain).

1. `repose-admin hosts mark-lost host-NN`: every project on it goes to
   `error` with reason `host_lost`, tenants get an email.
2. For each project: `repose-admin projects restore <id> --latest --to
   <other host>`. Data since the last snapshot (up to 24 hours, or since
   the last `stop`) is lost; the email says so. Tenants' git remotes hold
   whatever their agents pushed.
3. `repose-admin hosts retire host-NN`; `tofu apply` with it removed.

## Suspected cross-tenant access

1. `repose-admin hosts drain --all` (no new placements anywhere).
2. Snapshot the guests involved (`projects snapshot`); do not stop them
   yet, memory state may matter.
3. Capture: `journalctl -u hostd --since <window>` on the host, gateway
   logs from Loki for the window, `audit_log` for the window, nftables
   counters (`nft list ruleset`), `tcpdump` on the involved taps for the
   next hour.
4. Confirm or refute with the `test/isolation` suite against that host.
5. If confirmed: stop the offending guest, suspend the user, notify the
   affected tenant within 72 hours with what was reachable, write
   `docs/incidents/<date>.md`, fix, re-run isolation tests fleet-wide
   before undraining.

Who is told, what is captured, how a tenant hears (workstream 14 §5):

- **Told, in this order:** the owner (ntfy, then the incident file); the
  affected tenants; a third party only if their credentials inside the
  guest could have been read (GitHub, OpenAI, Anthropic) so they can
  rotate on their side. Nobody else until the timeline is written.
- **Captured before anything is stopped**, into
  `/var/lib/repose/incident-<date>/` on the host (root, 0700), then copied
  off with `scp` through the edge:
  ```
  journalctl -u hostd --since '<window start>' -o json > hostd.json
  journalctl -u sshd --since '<window start>' -o json > sshd.json
  journalctl -t hostd-audit --since '<window start>' -o json > audit-login.json
  nft list ruleset > nft.txt; bridge fdb show br br-guests > fdb.txt
  bridge -d link show > taps.txt; ip -s link > ifstats.txt
  hostd guests > guests.txt; hostd state export > state.json
  timeout 3600 tcpdump -nn -e -i tap-<8hex> -w tap-<8hex>.pcap &
  ```
  On the edge: gateway journal for the window; Loki
  `{component="gateway"}` and `{component="hostd", host="<id>"}` exports;
  `audit_log` rows for the window (`repose-admin audit --since`). Never
  capture guest disks or terminal contents beyond what the boundary test
  needs: an incident does not suspend the privacy policy.
- **Confirm or refute** with the suite, from the operator machine:
  ```
  REPOSE_ISOLATION_HOST_ID=<id> REPOSE_ISOLATION_EXEC_A='ssh -J root@<edge>,root@<host> dev@<A ip>' \
  REPOSE_ISOLATION_EXEC_B='ssh -J root@<edge>,root@<host> dev@<B ip>' \
  REPOSE_ISOLATION_HOST_EXEC='ssh -J root@<edge> root@<host>' \
  REPOSE_ISOLATION_A_IP=<A ip> REPOSE_ISOLATION_B_IP=<B ip> REPOSE_ISOLATION_A_MAC=<A mac> \
  REPOSE_ISOLATION_B_TAP=tap-<8hex> REPOSE_ISOLATION_HOST_IP=<.1> REPOSE_ISOLATION_OTHER_GUEST_IP=<other /22> \
  REPOSE_ISOLATION_A_GUEST_ID=<A guest id> REPOSE_ISOLATION_A_SLUG=<A slug> \
    go test ./test/isolation/ -run . -v -count=1 2>&1 | tee isolation-<date>.txt
  ```
  The output (host id and date on every test) goes into the incident
  file verbatim.
- **Tenant notice**, within 72 hours of confirmation, by email from the
  owner's address (the notification pipeline is for agent events, not
  incidents), one message per affected user, plain text: what boundary
  failed, the window, what was reachable in their environment (network
  ports, files, secrets by name only), what we did, what they should
  rotate, and a contact. Keep a copy in the incident file. A user whose
  data was *not* reached is not written to; say so in the file.
- **Incident file** `docs/incidents/YYYY-MM-DD.md`: timeline (UTC),
  boundary and mechanism, hosts and guests involved by id, what was
  captured and where it is, tenants notified and when, the fix, the
  re-run's `isolation-<date>.txt`, and the decision entry if a contract
  changed.

## OpenTofu state lock stuck

`make plan` or `make apply` reports `Error acquiring the state lock` with an
ID. An apply was killed and its blob lease survives it.

1. Confirm nobody is running an apply: ask, and check the CI run list.
2. `make -C infra force-unlock ENV=prod LOCK_ID=<id from the message>`.
3. If the state itself is wrong rather than locked, the container has blob
   versioning: `az storage blob list --account-name reposetfstate3912
   --container-name tfstate --include v` lists the versions and
   `az storage blob copy start` from one restores it.

A lease expires on its own after fifteen minutes; force-unlock is for when
waiting is not acceptable.

## An apply wants to replace a host data disk or a static IP

The plan shows `# forces replacement` on `azurerm_managed_disk.data`,
`azurerm_virtual_machine_data_disk_attachment.data`, or any
`azurerm_public_ip`. It cannot: those carry `prevent_destroy` and the plan
fails instead.

That is the intended outcome. A replaced data disk loses every tenant volume
on that host; a replaced static IP breaks every host's WireGuard endpoint and
the DNS that users type. Find what changed (usually `zone`, `disk_size_gb`
shrinking, or `storage_account_type`) and change it back. If the replacement
really is wanted, drain the host first
(`infra/README.md`, "Draining and destroying a host").

## Control plane cannot reach the edge network

Prometheus cannot scrape hosts, or the api cannot reach the edge.

1. `ssh root@<control ip> wg show`. No `wg0`: the edge's public key was not
   known when the VM was built.
2. `ssh root@<control ip> cat /etc/wireguard/publickey` and add it as a peer
   on the edge with allowed-ips `10.255.255.1/32`.
3. Put the edge's own public key in `prod.local.tfvars` as
   `edge_wireguard_public_key`, `make -C infra apply ENV=prod`, then
   `ssh root@<control ip> /usr/local/sbin/repose-wg-setup`.

The control plane's WireGuard private key is generated on the machine and
never leaves it, which is why this is two moves rather than one apply.

## hostd: join token already used

`systemctl status hostd` shows `status=3` and the unit is not retrying
(`RestartPreventExitStatus=3`); the journal says `register: join token
already used`.

1. The token was consumed by an earlier registration attempt that did not
   finish writing `/var/lib/repose/hostd/{cert,key}.pem`, or the host was
   re-imaged with the same cloud-init payload.
2. Mint a new token: `repose-admin hosts add --reissue <host>` (or `hostdev
   init` output on a hostdev-driven host), write it to
   `/run/repose/join-token`, `systemctl restart hostd`.

## hostd: api unreachable

`repose_host_stream_connected` is 0 and the journal loops on
`stream_disconnect`. Guests keep running; nightly snapshots still run
locally from `repose-snapshot.timer`.

1. `hostd status` (control socket) shows `stream_connected: false`.
2. From the host: `curl -sv https://api.repose.herakraft.co:443` (or the
   hostdev address). A TLS error naming the client certificate means the
   host certificate expired without rotation: `journalctl -u hostd | grep
   rotate`; rotation needs the stream, so if the certificate is past
   expiry re-register with a new token (previous entry) after moving the
   old `cert.pem` aside.
3. Results, events and up to 60 minutes of samples are buffered and sent
   on reconnect; longer outages lose samples (`repose_host_samples_dropped_total`).

## hostd: thin pool over 90 percent

`host_warning{pool_high}` at 80 percent, and CreateGuest and Restore return
`insufficient_capacity: thin pool 9x% full` from 90. See "PoolFull" above
for extending the pool; existing guests keep running throughout.

## hostd: store over 80 percent

`host_warning{store_high}` and Build refuses with `insufficient_capacity:
host store full`. See "StoreFull" above.

## hostd: guest never sends Ready

Create or Start fails with `guest_unresponsive: create: step 10 (ready)
failed: guest did not become ready` after 60 s; the guest is in `error`
and its tap, tc, nft membership and units are gone; the volume stays.

1. `/var/lib/repose/guests/<id>/console.log` holds the boot output. No
   output at all: the kernel or initrd path in `ch.args` is wrong (the
   closure is not a bootable system) or KVM is missing.
2. A boot that stops at mounting `/nix/.ro-store`: virtiofsd died; `journalctl
   -u virtiofsd@<id>`.
3. A boot that reaches login but never Ready: guestd is not running in the
   guest; the base image is at fault (workstream 02).
4. Fix, then `repose-admin projects start <id>` (or `hostdev start`).

Since I-62 a virtiofsd that exits before creating its socket fails the
create at step 8 instead; on an older hostd, `systemctl status
virtiofsd@<guest id>` and `journalctl -u guest@<guest id>` (Cloud
Hypervisor "Failed connecting the backend ... virtiofsd.sock") are the
first things to read when this message appears at once after a create.

## hostd: virtiofsd exited under a running guest

The guest is in `error` with reason `virtiofsd exited`; hostd stopped the
hypervisor cleanly because the guest would see I/O errors on every store
path. `journalctl -u virtiofsd@<id>` for the cause (usually OOM against
its 1 GB MemoryMax, or a chroot problem after a store bind-mount change).
`projects start` brings it back; the api restarts once on its own.

## hostd: hypervisor exited unexpectedly

Reason `hypervisor exited <code>` on the guest; tap and tc are torn down.
`journalctl -u guest@<id>` and the tail of `console.log`. Code 137 is the
`MemoryMax` cgroup limit (class RAM plus 512 MB) killing CH: the guest
used more than its class through virtiofsd cache pressure; a larger class
or a base bug. The api restarts once automatically, then leaves it in
error with an event.

## hostd: freeze without thaw

`Warning{freeze_timeout}` from the guest and a failed snapshot: hostd lost
the vsock connection or crashed between `Freeze` and `Thaw`; guestd thawed
itself after 10 s so the tenant saw at most a 10 s pause. The snapshot
is marked failed and retried by the api's timer; nothing else to do
unless it repeats, in which case check `repose_host_snapshot_freeze_seconds`
for a slow `lvcreate -s` (pool metadata nearly full).

## hostd: snapshot upload failed

Result `internal: snapshot upload failed: <err>`; the LVM snapshot was
removed regardless. A 403 means the managed identity lost its role on the
`repose-snapshots` container (`az role assignment list`); a timeout means
egress from the host is broken (NAT gateway). The api retries once from
its timer; `SnapshotStale` fires if a running project stays without one.

## hostd: build exceeds time

`build_timeout: build timed out after 30 minutes while building <name>`
and the CLI exits 10. The named derivation is what was compiling when
`timeout` fired (the scope's `RuntimeMaxSec` is the backstop 30 s later);
the tenant either pulls a cached variant or accepts the cap. "Build
stuck" and `BuildQueueStuck` above cover a build that ignored the cap.

## Build stuck (no BuildLog line for 10 minutes)

A `Build` op is `running`, its SSE log has not moved, and hostd's
`builds_running` gauge holds. Distinguish a slow build from a wedged one:

1. On the host: `systemctl list-units 'repose-build-*'` shows the scope
   (`repose-build-<revision>` for the build, `-eval` for the evaluation)
   with its `RuntimeMaxSec`; `systemctl status <scope>` shows the `nix`
   process tree under user `nixbuild`. A build is alive when `nix log
   --follow` on its derivation (`journalctl -u hostd | grep build_start`
   names the revision; `nix log <drv>`) still grows.
2. A build compiling something big (a browser, CUDA) is slow, not stuck;
   the 30-minute cap ends it as `build_timeout` naming the derivation and
   the tenant reads it in the CLI. Nothing to do.
3. If the scope is past `RuntimeMaxSec` and still present, systemd's kill
   failed: `systemctl kill --signal=KILL <scope>`; hostd reports
   `build_timeout`. If the `nix` client is gone but `nix-daemon` still runs
   the builder (`ps -o pid,user,etime,args -C nix-daemon`), the daemon lost
   the client's cancellation: `nix build --no-link <drv>` from a root shell
   attaches to the running build so you can watch it, or `systemctl restart
   nix-daemon` kills every build on the host (both hostd builds restart
   from their last substituted path; nothing tenant-visible is lost).
4. `BuildQueueStuck` above covers the metric-level alert.

## Build: cache unreachable

`host_warning{kind: cache_unreachable}` and BuildLog lines `warning:
unable to download 'https://<cache>/...'`. Builds go on from
cache.nixos.org or source; only speed is lost.

1. `curl -sI https://repose.cachix.org/nix-cache-info` from the host. A
   DNS or TLS failure from the host and not from your laptop is the host's
   egress (NAT gateway, `HostWgDown` is unrelated). A 5xx is Cachix; check
   status.cachix.org and wait.
2. If the cache is fine but every host warns, the public key changed:
   `repose.host.overlayCache.publicKey` must equal the key on the cache's
   page, else Nix refuses its narinfos with `signature ... invalid` and
   falls back to building the agents from their release tarballs
   (downloads, not compiles; minutes).
3. `nix store info --store https://repose.cachix.org` proves the host
   reaches it after the fix. The warning is per build and stops by itself.

## Build: base unavailable

`Build` fails `internal: base <rev> unavailable: ...` for every project;
no tenant config changes.

1. `ls /var/lib/repose/base/` on the host. Each directory is a git
   checkout of this repository at a `base_versions.nix_rev`.
2. `clone failed`: hostd's `--base-repo-url` is empty or the repository
   refused it. A private repository needs `repose.host.baseRepo.sshKeyFile`
   pointing at a deploy key delivered like the join token (never in the
   store); `journalctl -u hostd | grep base` has git's stderr. Place the
   checkout by hand from a machine that can: `git clone --no-checkout
   <url> /var/lib/repose/base/<rev> && git -C /var/lib/repose/base/<rev>
   checkout <rev>`, then resend the build (`repose-admin projects
   rebuild <id>`, or `repose config apply` as the user).
3. `checkout failed`: the revision is not in the repository (a
   `base publish` of a rev that was never pushed). Publish a rev that
   exists.
4. `flake.lock` errors: the checkout is not a complete repository
   (interrupted clone). Remove the directory and let hostd clone again.

## Base bump failures

`repose-admin base status <version>` lists projects `failed` with the
error's first line; each has a `base_update_failed` event the user saw.

1. One or two projects failing with `eval_failed` or `build_failed` is a
   fragment that stopped building on the new base (a renamed attribute, a
   removed package). The project keeps its old base and is skipped by
   later bumps until the user's next successful `repose config apply`;
   nothing to do on the platform side, but the error text tells you which
   nixpkgs change caused it.
2. Many projects failing the same way is the base's bug: `repose-admin
   base rollback <previous version>` re-applies the previous closure to
   every project the bump reached (still rooted, so no rebuild), then fix
   the base and publish again.
3. `needs_reboot` is not a failure: the kernel changed, the guest was
   built and the user decides when to reboot (`repose config apply
   --reboot`); `repose status` says `base X (Y ready; reboot when
   convenient)`.
4. A bump that never started: the daily job runs at 04:00 UTC (05); a
   `--security` publish runs at once. `repose-admin ops list --kind build`
   shows the queued builds and their `not_before` times, spread over 24 h
   (2 h security), two per host at a time.

## Closure collected under a running guest

`Warning{kind: store_path_missing}` from a guest, or a tenant's shell
saying `No such file or directory` for a store path. The host garbage
collected a path the guest's running system references, which means its
GC root was missing (a hostd bug) or was removed by hand.

1. `ls -l /nix/var/nix/gcroots/repose/` on the host: the guest's root is
   `<guest_id>` and the newest three revisions are
   `rev-<project_id>-<revision_id>`; `nix-store --gc --print-dead | grep
   <closure>` is empty when the closure is protected.
2. Rebuild the revision (deterministic): `repose-admin projects rebuild
   <id>` sends `Build` with the same fragment and base; hostd re-roots the
   result. A guest that only lost a leaf package keeps running from page
   cache and picks the path up at its next `switch`; one that lost its
   `init`, kernel or a service binary is wedged: `repose-admin projects
   restart <id>` (stop with a snapshot, start) boots it from the rebuilt
   closure.
3. Find why the root was gone: `journalctl -u hostd | grep gcroot`
   (hostd logs every root it sets and removes) and the weekly
   `nix-gc.service` run time. A root removed by hand is an audit finding.

## hostd: command for unknown guest

`not_found: guest <id> not on this host`. The api's placement and the host
disagree, usually after a restore onto another host that was not recorded.
`repose-admin hosts reconcile <host>` reads the host's Hello and fixes the
api's view.

## hostd: state.db corrupt or lost

hostd refuses to start and logs the path. Move the file aside and run
`hostd reconcile --rebuild` (alias `--from-api`): it rebuilds the guest
table from `/var/lib/repose/guests/*/guest.json`, `lvs` and
`systemctl list-units 'guest@*'`, then the next Hello lets the api
reconcile its own view. `hostd guests` must match `lvs vg-guests` and the
unit list afterwards. Command results are lost, so the api may re-send
commands; every command but Exec re-executes safely.

## hostd: two hostd processes

The second prints `hostd already running` and exits 1: bbolt holds a
file lock on `state.db`. Nothing to do; `systemctl status hostd` shows
the real one.

## api: login service unavailable

The CLI prints `login service unavailable, retry in a minute`; the api
logs `request` lines with status 401 and the message `identity provider
unavailable`. Logto's JWKS could not be fetched and the cached copy is
older than 24 hours (fresh copies are served for up to an hour without a
fetch).

1. `curl -s https://auth.repose.herakraft.co/oidc/jwks` from the Coolify
   VM. Logto down: its container in Coolify. A 200 here means the api
   container cannot reach it (DNS inside the Coolify network).
2. The api recovers on the next request once the fetch succeeds; nothing
   to restart.

## api: could not create your account

A first sign-in failed with `internal: identity provider unavailable`
and no user row exists. The Management API call
(`GET <issuer>/api/users/<sub>` with the M2M client) failed.

1. Check `LOGTO_M2M_CLIENT_ID/SECRET` on the api app and that the M2M app
   in Logto holds the Management API `all` scope.
2. The user retries; the api creates the row on the first successful
   lookup. Nothing is half-created.

## api: no capacity right now

An op ended `capacity`; `repose_api_schedule_total{result="capacity"}`
rose; the CLI printed `no capacity right now; you have not been charged`.

1. `repose-admin hosts list`: no host is `ready` with free memory above
   the reserve for the class and pool room for the volume. Draining,
   unreachable and stale-heartbeat hosts do not count.
2. Add a host (11 §5) or undrain one. The user runs `repose run` again;
   the project is in `error` with `last_error = capacity` until then.

## dashboard: up but the "Cannot reach the API" bar is showing

The dashboard itself is fine (its `/healthz` is separate from the api's);
the bar (08-dashboard.md 5.5/6) means the browser's last request to
`PUBLIC_API_URL` either failed at the network level or came back 5xx.

1. Check the api from outside the browser: `curl -i
   https://api.repose.herakraft.co/v1/healthz`. A non-200 or a hang points
   at the api's own runbook rows above (Postgres down, Key Vault
   unavailable) rather than the dashboard.
2. If that curl is fine, it's CORS or the wrong origin: open the browser
   console for a `blocked by CORS policy` message, and check
   `PUBLIC_API_URL` in the dashboard's Coolify environment matches the
   api's real origin exactly (scheme and host). The api sends
   `Access-Control-Allow-Origin: *` on every `/v1` route (I-79); a proxy
   or CDN in front of it that strips that header reproduces this exact
   symptom.
3. The bar clears on its own once a poll succeeds; polling backs off to
   60 s while it thinks the api is unreachable (5.3), so a fixed api can
   take up to a minute to clear the bar, or reload the page to force an
   immediate recheck.

## dashboard: sign-in loop

The browser bounces `/` → `/callback` → `/` without ever reaching
`/projects`, or keeps landing back on `/` after visiting a page while
signed in.

1. Open the browser console during the loop. `handleSignInCallback` failing
   (auth.svelte.ts) means either the code was already consumed (a page
   refresh on `/callback`, or a browser prefetch hitting it twice — see
   `data-sveltekit-preload-data="hover"` in app.html) or `PUBLIC_LOGTO_APP_ID`
   / `PUBLIC_LOGTO_ENDPOINT` don't match the Logto application the CLI and
   dashboard were both registered under.
2. If it loops without ever reaching `/callback` at all, `signIn()` itself
   threw — almost always `PUBLIC_LOGTO_ENDPOINT` unset or unreachable from
   the browser (check it the same way as the api origin above; Logto needs
   the same cross-origin discovery fetch the api does).
3. A user stuck signed in on `/` (authenticated but not redirected to
   `/projects`) points at the root `+layout.svelte` effect instead: confirm
   `authState.authenticated` actually resolves (it stays `undefined`
   forever if `initAuth()` threw), not a Logto problem.

## api: op stuck waiting for host

`GET /ops/:id` stays `running` and the CLI shows `waiting for host`. The
host's stream dropped mid-command.

1. `repose-admin ops list --state running` and `hosts list`: the host's
   heartbeat age. Under 90 s: hostd reconnects with backoff and the api
   re-sends the command on `Hello` with the same command_id
   (`command_resend` in the log); nothing to do.
2. Unreachable for more than 10 minutes: the op fails
   `host_unreachable`, the project goes to `error`. See
   "HostUnreachable"; `repose-admin projects start` once the host is back.

## api: build failed

The revision is `failed` with the Nix error and `fragment_line`; the CLI
exits 10. The guest is untouched and the previous revision stays applied.
`repose-admin ops log <op>` has the full output (`build_logs`, secret
values already redacted). A fragment that contains a current secret value
is refused before the build with `invalid: fragment contains the value of
secret NAME`.

## No notifications arriving

A user reports nothing on their phone or in their inbox for an agent that
clearly finished.

1. `repose status` / `GET /projects/:id/events` first: if the event is not
   there at all, the problem is upstream of the outbox (the hook never
   fired, or the guest never reached the api). Check `guestd_ok` in the
   project's signals (`GuestdLost` if it is false) and, on the guest,
   whether `/run/repose/hooks.sock` exists and the agent's wrapper actually
   ran `repose-agent-setup` (`grep repose-hook` in the agent's own hook
   config file, `guest-conventions.md` "Agent wrappers").
2. If the event is there but `delivered` has no key for the channel: the
   outbox has not picked it up yet, or the channel is disabled
   (`notify_email` false, `ntfy_url` null) or over the 30/hour rate cap
   (`kind = 'notifications_paused'` events on the project in the last
   hour). `repose_api_outbox_depth` and `repose_api_outbox_lag_seconds`
   rising together mean the worker itself is stuck: it holds
   `db.LockOutbox`, so `select pg_advisory_lock_...` on the wrong replica
   or a stuck transaction is what to look for; only one replica runs it
   (`api: rollup or expiry not running on one replica` is the same shape
   for a different job).
3. If `delivered[channel]` says `"error: ..."` or `"failed: ..."`, the
   channel-specific entries below have the fix. Nothing to do if it says a
   timestamp: delivery succeeded and the miss is client-side (a stale
   ntfy subscription, a spam folder).

## ntfy failing

`delivered.ntfy` carries `"error: ..."` (retrying) or `"failed: 404"` (not
retried, DECISIONS §5's 4xx rule) and the settings page shows a warning.

1. A 4xx (`400`, `404`) means the URL is wrong or the topic does not exist
   on that server any more: the user re-pastes it from
   `repose notify set ntfy <url>`, which sends a test push
   (`POST /me/notify-test`) so a wrong URL is caught immediately rather
   than on the next real event.
2. A 5xx or timeout retries on the schedule in `13-notifications.md` §5.5
   (7 attempts over roughly 24 h) before `failed`; a self-hosted ntfy
   server that is down for longer than that needs the user to re-set the
   URL once it is back, which requeues nothing retroactively — only new
   events are affected.
3. `repose_api_notify_total{channel="ntfy",result="failed"}` rising across
   many users means a widely-used relay (`ntfy.sh`) is down, not a
   per-user URL problem; check its status page before debugging further.

## Resend failing

Every email `delivered.email` value is `"error: ..."` or `"failed: ..."`
across many users at once (a per-user failure is `email: "user has no
address"`, permanent, and not a Resend outage).

1. Resend's status page and `RESEND_API_KEY`'s validity
   (`repose-admin` has no direct check; a `401` in the api's logs under
   `event=notify_fail` for the email channel is the tell — Resend's API key
   is a Coolify secret, `ops/coolify/api.env.example`).
2. A `429` retries like any 5xx (the sender treats both as retryable); a
   sustained `429` means the account's Resend rate limit needs raising.
3. `repose_api_notify_total{channel="email",result="failed"}` over 5
   percent in 10 minutes is `13-notifications.md` §6's alert threshold;
   page on it rather than waiting for a user to report silence.

## api: secret service unavailable

`PUT /secrets` returned `internal: key service unavailable`;
`repose_api_keyvault_errors_total` rose. Guest starts retry for 30
minutes (`op_retry` in the log) before failing with the same message.

1. `az keyvault key show --vault-name <kv> --name repose-dek-kek` with the
   api's identity; a 403 means the access policy lost the api's object id
   (11 §Key Vault); a timeout means egress from the Coolify VM.
2. Reads keep working from the 10-minute DEK cache; writes and cold starts
   do not. Nothing to restart once Key Vault answers.

## api: Postgres down

`/healthz` returns 503 (`db unreachable` or `migrations pending`) and
Coolify stops routing; host streams drop and hostd buffers samples for 60
minutes. Fix the database (Coolify's Postgres resource, disk, or run
`repose-admin db migrate` for the pending case); the api needs no restart.

## api: rollup or expiry not running on one replica

`rollup: not leader` (`rollup_skip`) means another replica holds the
advisory lock; only one runs the hourly rollup, the outbox, snapshot
expiry and the ops driver. Expected with two replicas. If no replica logs
`rollup_done` for two hours, `RollupLag` fires: `repose-admin billing
rollup` runs it by hand.

## api: snapshot deleted under a restore

Cannot happen by construction: the restore op marks
`snapshots.restoring_op_id` inside the same row lock the expiry job takes
with `for update skip locked`, and the job skips rows a restore holds. If
a restore fails with `not_found: snapshot was deleted`, the snapshot had
expired before the restore was enqueued; pick a newer one.

## api: duplicate results or events

Ignored by design: `ops.command_id` is unique and a result for a finished
command logs `duplicate or unknown result ignored`; `events.host_event_id`
is unique and hook events collapse on `(project, agent, kind, second)`.
Nothing to do.

## api: user reports "account suspended"

`forbidden: account suspended` on every route but `GET /me` and the
billing portal. `repose-admin users show <handle>` for the reason;
`users unsuspend` reverses it.

## Suspend a user

`repose-admin users suspend <handle> --reason "..."`: stops all their
guests (with snapshot), sets `billing_status = suspended`, revokes their
certificates, and writes an audit row. Their data is retained on the normal
30-day schedule from the moment of suspension unless `--retain` is passed.

## CLI: user cannot log in

`repose login` never completes: the browser flow times out after 5
minutes, or device code polling reports `authorization_pending` forever
and then `device code expired`.

1. Ask which path they used. Loopback (PKCE): a corporate firewall or a
   browser extension can block `http://127.0.0.1:<port>/callback`;
   `--no-browser` (or `REPOSE_NO_BROWSER=1`) forces device code, which
   only needs outbound HTTPS.
2. `curl -s https://auth.repose.herakraft.co/oidc/.well-known/openid-configuration`
   from the user's machine: a failure here means the CLI cannot even
   start (`Cannot reach <issuer>`, exit 1), independent of the flow.
3. Confirm the `repose-cli` Native application in Logto still has the
   loopback redirect (`http://127.0.0.1:*/callback`) and device flow
   enabled (`ops/AZURE-SETUP.md` step 12); a removed or misconfigured app
   answers `invalid_client` on the token exchange.
4. `repose logout --purge` clears any half-written `credentials.json` or
   `~/.ssh/repose/` state before retrying.

## CLI: certificate rejected

`repose run`/`attach`/`open` gets a gateway banner (`certificate not
valid for this project`, or a stopped-project banner) instead of a shell,
or `POST /certs` itself fails.

1. `repose certs`... there is no such command; the certificate lives at
   `~/.ssh/repose/id_ed25519-cert.pub`. `ssh-keygen -L -f
   ~/.ssh/repose/id_ed25519-cert.pub` shows its principals and expiry —
   confirm the failing project's id is in `Principals` and `Valid` has
   not passed (12h lifetime, R3-9).
2. A missing or expired principal means the cert predates the project
   (created on another machine, or before this project existed locally):
   delete the cert file and re-run; `ensureCert` reissues one covering
   every project the account has.
3. `POST /certs` answering `rate_limited` (10/min) is expected under
   rapid repeated runs; the CLI reuses the certificate on disk if it still
   has validity and only warns. If none is on disk yet, wait a minute.
4. A banner that is not one of the two above (gateway-side rejection, not
   yet built as of 07-cli.md's landing — see 06-gateway-edge) surfaces
   verbatim with exit 1; file it against the gateway, not the CLI.

## CLI: SSH timeout after running

`Guest is running but SSH did not answer in 60s.` after the API already
reports the project `running`.

1. `repose logs --kind console` — sshd not started yet (guest still
   booting past `running`), or a boot failure, both show here.
2. `repose status` for `sessions`/`tmux clients`: if the API's state is
   stale (host lost contact), `hosts` on the host-01 side and
   `repose-admin hosts show` tell you whether the host itself is
   reachable; a `sessions` count that never reflects change means the
   API's view of the guest is the stale part, not SSH.
3. Retry `repose run`/`attach` once more before escalating: the 60s
   window is deliberately short so an agent's user is not left staring at
   a hang; the guest is very likely still coming up.

## StripePushBacklog

`repose_api_billing_stripe_push_backlog_seconds` past six hours: the
oldest `usage_hours` row with no `stripe_usage_record_id` is that old.
`StripePushFail` catches a push that errors; this catches the quiet cases
where nothing errors and nothing ships.

Look in this order:

1. Is Stripe configured at all? The api logs `billing_disabled` at start
   when `STRIPE_SECRET_KEY` is unset, and every billing route answers
   `503 billing_disabled` (DECISIONS I-16). Rows keep accruing and are
   pushed once it is set; nothing is lost.
2. Is the rollup running? `RollupLag` and `api: rollup or expiry not
   running on one replica` above.
3. Is Stripe refusing? `stripe_push_fail` lines carry the error. A key
   that was rotated or a meter that was deleted are the two that produce a
   steady failure.

Then `repose-admin billing resync`, which re-pushes every pending row and
prints how many moved. The push is idempotent twice over: a row with a
record id is never re-sent, and the meter event identifier
(`usage:<project>:<hour>:<part>`) is unique at Stripe's end as well, so a
resync cannot double-bill.

## BillingMismatch

`repose_api_billing_mismatch_cents` above zero: the nightly reconciliation
found `usage_hours` and Stripe disagreeing for at least one account. The
job never fixes a difference, so the alert stays lit until a human acts.

```
repose-admin billing reconcile              # the current period, per account
repose-admin billing reconcile --month 2026-10
repose-admin billing explain <project> <2026-10-04T13>
```

The `UNPUSHED` column is the usual innocent explanation: rows that have
not reached Stripe yet, which is `StripePushBacklog`, not a mismatch of
substance. A difference that is not explained by unpushed rows means one
of the two ledgers is wrong:

- **usage_hours is right, Stripe is short.** Re-push with `billing
  resync`. If the rows already carry record ids and Stripe still has not
  got them, the identifiers were consumed by an earlier push that failed
  after Stripe accepted it; issue the difference as an invoice item in the
  Stripe dashboard rather than clearing the record ids.
- **Stripe is right, usage_hours is wrong.** Do not edit `usage_hours`:
  it is the ledger of record for what was used, and an invoice already
  refers to it. Correct the customer with `repose-admin billing credit
  <handle> <cents> "<reason>"`, which is what the credit ledger is for.

Either way, nothing here is automatic and nothing is silent.

## A user says they were overcharged

`repose-admin billing explain <project> <hour>` prints every input and each
step of the pricing rule for one hour: the samples the hour was built from,
the period running totals before it, which cap applied and why, the storage
remainder, the credit taken and what reached Stripe. Walk the hours they
question; the arithmetic is the answer.

The three that come up:

- **"I stopped it and it still charged me."** Storage accrues for as long
  as the project exists, on the *allocated* volume size (`PRICING.md`). The
  `storage` line in `explain` shows it; the `guest` line will be zero.
- **"It says more hours than I used."** A guest-hour is a minute of
  `running` samples; `explain` prints how many samples the hour had. If it
  shows fewer samples than seconds implies, that is the gap case and it
  under-bills, never over.
- **"I was charged after I hit the cap."** The cap is per project per
  billing period and applies to guest-hours only; storage and egress are
  always additive (§5.1). `explain` names the cap class and the period
  total, which is where a mid-period class change shows up.

A correction is a `repose-admin billing credit` row, never an edit to
`usage_hours`.

## StripeWebhookRejected

Five or more deliveries in ten minutes failed signature verification
(`stripe_webhook` lines with `result=bad_signature`; the body is never
logged). Two causes, in order of likelihood:

1. **The endpoint's API version does not match the deployed SDK.**
   `stripe-go` refuses an event rendered under another version, because an
   object it deserialises wrongly is a wrong amount. `go doc
   github.com/stripe/stripe-go/v83.APIVersion` prints what the binary
   expects; the endpoint's version is on its page in the Stripe dashboard.
   Recreate the endpoint on the right version (`ops/AZURE-SETUP.md`
   step 17). This is the one that appears right after an SDK upgrade.
2. **`STRIPE_WEBHOOK_SECRET` is not this endpoint's signing secret.** A
   second endpoint, or a test-mode secret on a live-mode deployment.

While it fires, no invoice event is applied: accounts will not move to
`past_due` or back to `active`. Stripe retries for up to three days, so
fixing the secret inside that window replays everything; past it, resend
the events from the endpoint's page in the dashboard.

If neither is true, someone is posting at the endpoint. It is
unauthenticated by design (the signature is the authentication) and a
forged body cannot pass, so this is noise rather than an incident; the
rate limit in front of the api is the answer if it becomes constant.
