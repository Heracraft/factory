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
repose-admin login                          # Logto, operator role
repose-admin operator-cert                  # 8h Host-CA cert into ssh-agent
sudo wg-quick up ops/wg/operator.conf        # 10.255.0.0/16 reachable
```

### Control plane (Coolify VM)

1. `tofu -chdir=infra/azure/prod apply` with `coolify_vm = true`. Cloud-init
   installs Coolify; open `https://<ip>:8000`, create the admin user, add the
   server as `localhost`.
2. Add Logto as a Docker Image resource (`ghcr.io/logto-io/logto`), Postgres
   as a Coolify database, run its migration, set `ENDPOINT` and
   `ADMIN_ENDPOINT` to `auth.repose.herakraft.co`. Configure the GitHub
   connector and two applications: `repose-cli` (Native, device flow on)
   and `repose-web` (SPA). Create API resource
   `https://api.repose.herakraft.co`.
3. Add `api` and `web` as Dockerfile applications from the repo, health
   checks `/healthz` and `/`, env from `ops/coolify/api.env.example`.
   Secrets (CA keys, Key Vault client cert, Stripe keys, Resend key) as
   Coolify secrets.
4. Add the platform Postgres as a Coolify database with S3 backup to R2
   (`infra/r2` outputs), nightly 02:00, retention 35 days. Run
   `repose-admin db migrate`.
5. Add the Coolify VM as a WireGuard peer of the edge (`ops/wg/coolify.conf`).

### Edge

`tofu apply` with `edge = true`; nixos-anywhere installs `.#edge`. Then
`repose-admin edge init` writes the WireGuard hub key into the api and the
gateway's mTLS client cert. Verify `ssh -p 22 probe.nobody@ssh.repose.herakraft.co`
returns `certificate required`.

### First host

See `../workstreams/11-infra-opentofu.md` §5 "Adding a host". Then
`repose-admin hosts list` shows `ready`, and `repose-admin hosts smoke
<host>` creates, starts, snapshots and destroys a throwaway guest.

### Observability

On the personal server: add `ops/prometheus/repose.yaml` to Prometheus's
scrape configs, `ops/alerts.yaml` to its rule files, `ops/dashboards/*.json`
to Grafana provisioning, and the Loki labels are already in Fluent Bit.

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

1. Grafana gateway dashboard: failures by `reason`. `bad_cert` in volume
   from one source is a scan; the gateway rate-limits per source IP after
   20 failures (fail2ban-style, built in). Nothing to do unless it
   persists for hours; then add the source to the edge NSG deny list.
2. `expired` in volume means the CLI's silent refresh is broken for many
   users: check `repose_api_certs_issued_total` fell off a cliff, and the
   Logto token endpoint.
3. `wrong_principal` from one user repeatedly is someone probing other
   projects; `repose-admin audit --user`.

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
2. The host's `inet repose` `input` chain sends every frame from
   `br-guests` to `guest_in`, which drops all but rate-limited ICMP, so a
   TCP connection the host opens to a guest never gets its replies. For the
   length of the session, and only on a host driven by `hostdev`, admit
   replies to host-initiated flows:
   `nft insert rule inet repose input iifname "br-guests" ct state established,related accept`.
   The rule is runtime only: a `systemctl reload nftables` or a reboot
   removes it. Guests still cannot open anything towards the host.
3. `hostdev ssh-cert --project <p> --pubkey ~/.ssh/id_ed25519.pub >
   ~/.ssh/id_ed25519-cert.pub` on the edge, then
   `ssh -J root@<edge ip>,root@<host ip> dev@<guest ip>`.

Remove the rule when done (`nft -a list chain inet repose input`, then
`nft delete rule inet repose input handle <n>`).

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
   `ro-store` tag: start it and restart the guest.
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

## Postgres restore

1. Provision a scratch Coolify (or use staging), add a Postgres database,
   download the newest dump from R2 (`rclone ls r2:repose-pg-backups`),
   `pg_restore` into it.
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

`build_timeout: build exceeded 1800 s; last derivation: <name>` and the
CLI exits 10. The named derivation is what was compiling when `timeout`
fired; the tenant either pulls a cached variant or accepts the cap.
`BuildQueueStuck` above covers a build that ignored the cap.

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

## Suspend a user

`repose-admin users suspend <handle> --reason "..."`: stops all their
guests (with snapshot), sets `billing_status = suspended`, revokes their
certificates, and writes an audit row. Their data is retained on the normal
30-day schedule from the moment of suspension unless `--retain` is passed.
