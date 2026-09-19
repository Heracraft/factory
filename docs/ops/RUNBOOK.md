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
| Add a host | `repose-admin hosts add --name host-NN` then `tofu apply` (workstream 11) |
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

## Host never registered

A new host has been up for more than five minutes and is not in `hosts
list`.

1. `ssh root@<private ip>` via the edge: `journalctl -u hostd`. `token_expired`
   or `token_used`: mint a new one (`repose-admin hosts add --reissue`),
   write it to `/run/repose/join-token`, `systemctl restart hostd`.
2. `kvm_missing`: the VM was created without `security_type = Standard`.
   Destroy and recreate; there is no in-place fix.
3. `pool_missing`: the data disk is not attached or `disko` did not run.
   `lsblk`; re-run nixos-anywhere if the layout is missing.

## Guest unresponsive

A tenant reports `repose attach` hangs, or `Freeze` times out.

1. `repose-admin projects show`: state and last signals. `guestd_ok=false`
   means "GuestdLost" above.
2. `repose-admin exec <id> -- uptime` (goes through vsock; if it works,
   the guest is fine and the problem is the gateway or the tenant's
   certificate).
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

## Suspend a user

`repose-admin users suspend <handle> --reason "..."`: stops all their
guests (with snapshot), sets `billing_status = suspended`, revokes their
certificates, and writes an audit row. Their data is retained on the normal
30-day schedule from the moment of suspension unless `--retain` is passed.
