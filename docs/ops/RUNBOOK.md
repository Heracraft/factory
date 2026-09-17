# Runbook

Symptom-titled. Each alert in `../CHECKLIST.md` and `../workstreams/10-observability.md`
has an entry with the same name. Commands assume an operator certificate
(`factory-admin operator-cert`) and the edge WireGuard up on the operator's
machine.

## First-time setup

### Operator machine

```bash
nix develop                                  # go, tofu, az, wg, promtool, grafana-cli
az login
factory-admin login                          # Logto, operator role
factory-admin operator-cert                  # 8h Host-CA cert into ssh-agent
sudo wg-quick up ops/wg/operator.conf        # 10.255.0.0/16 reachable
```

### Control plane (Coolify VM)

1. `tofu -chdir=infra/azure/prod apply` with `coolify_vm = true`. Cloud-init
   installs Coolify; open `https://<ip>:8000`, create the admin user, add the
   server as `localhost`.
2. Add Logto as a Docker Image resource (`ghcr.io/logto-io/logto`), Postgres
   as a Coolify database, run its migration, set `ENDPOINT` and
   `ADMIN_ENDPOINT` to `auth.factory.herakraft.co`. Configure the GitHub
   connector and two applications: `factory-cli` (Native, device flow on)
   and `factory-web` (SPA). Create API resource
   `https://api.factory.herakraft.co`.
3. Add `api` and `web` as Dockerfile applications from the repo, health
   checks `/healthz` and `/`, env from `ops/coolify/api.env.example`.
   Secrets (CA keys, Key Vault client cert, Stripe keys, Resend key) as
   Coolify secrets.
4. Add the platform Postgres as a Coolify database with S3 backup to R2
   (`infra/r2` outputs), nightly 02:00, retention 35 days. Run
   `factory-admin db migrate`.
5. Add the Coolify VM as a WireGuard peer of the edge (`ops/wg/coolify.conf`).

### Edge

`tofu apply` with `edge = true`; nixos-anywhere installs `.#edge`. Then
`factory-admin edge init` writes the WireGuard hub key into the api and the
gateway's mTLS client cert. Verify `ssh -p 22 probe.nobody@ssh.factory.herakraft.co`
returns `certificate required`.

### First host

See `../workstreams/11-infra-opentofu.md` §5 "Adding a host". Then
`factory-admin hosts list` shows `ready`, and `factory-admin hosts smoke
<host>` creates, starts, snapshots and destroys a throwaway guest.

### Observability

On the personal server: add `ops/prometheus/factory.yaml` to Prometheus's
scrape configs, `ops/alerts.yaml` to its rule files, `ops/dashboards/*.json`
to Grafana provisioning, and the Loki labels are already in Fluent Bit.

## Common operations

| Task | Command |
|---|---|
| List hosts with capacity | `factory-admin hosts list` |
| Add a host | `factory-admin hosts add --name host-NN` then `tofu apply` (workstream 11) |
| Drain a host (no new placements) | `factory-admin hosts drain host-NN` |
| Retire a host (after all projects moved) | `factory-admin hosts retire host-NN` |
| Move a project to another host | `factory-admin projects move <id> --to host-NN` (stop, snapshot, restore, start) |
| Suspend a user | `factory-admin users suspend <handle> --reason "..."` (stops guests, freezes billing, audit row) |
| Unsuspend | `factory-admin users unsuspend <handle>` |
| Run a command in a guest (audited) | `factory-admin exec <project-id> -- <argv>` |
| Force a snapshot | `factory-admin projects snapshot <id>` |
| Restore a snapshot | `factory-admin projects restore <id> --snapshot <sid> [--to host-NN]` |
| Revoke all certs for a user | `factory-admin certs revoke --user <handle>` |
| Rotate a host's mTLS cert | `factory-admin hosts rotate-cert host-NN` |
| Rotate the Key Vault wrapping key | `az keyvault key rotate` then `factory-admin secrets rewrap` |
| Query audit log | `factory-admin audit --user <handle> --since 24h` |
| Publish a base version | `factory-admin base publish --rev <git sha> --changelog "..." [--security]` |
| Re-run the hourly rollup | `factory-admin billing rollup --hour 2026-09-17T14` |
| Database migration | `factory-admin db migrate` / `factory-admin db rollback --to NNNN` |

## HostMemory80

Reserved memory on a host is above 80 percent.

1. `factory-admin hosts list`: confirm which host and how many guests are
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
   means the host cert expired: `factory-admin hosts rotate-cert` from the
   api side writes a new one via the edge jump.
3. If the VM is gone: "Host loss" below.
4. Guests keep running through an api outage; the only thing lost is
   metering samples for the window, which the rollup marks as `gap` rather
   than zero.

## SnapshotStale

A running project's newest snapshot is older than 36 hours.

1. `factory-admin projects show <id>`: last snapshot, last error.
2. `journalctl -u hostd | grep snapshot_fail` on the host. Common causes:
   `freeze_timeout` (guestd hung; see "Guest unresponsive"), Blob auth
   (managed identity lost its role: `az role assignment list`), pool out of
   metadata space (`lvs -a`; extend the pool metadata).
3. `factory-admin projects snapshot <id>` after fixing; confirm the age
   gauge drops.

## BuildQueueStuck

Two builds running on a host with no completion for 45 minutes.

1. `factory-admin ops list --host host-NN --state running`.
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
   users: check `factory_api_certs_issued_total` fell off a cliff, and the
   Logto token endpoint.
3. `wrong_principal` from one user repeatedly is someone probing other
   projects; `factory-admin audit --user`.

## EgressHigh

A project moved more than 1 TB in 24 hours.

1. Abuse dashboard: the project's `proc_samples` top `comm`. A torrent
   client, a scraper, or a miner's pool traffic looks different from
   `docker pull`.
2. If legitimate, nothing (it is metered and billed). If not:
   `factory-admin users suspend`.

## PoolFull

Thin pool under 10 percent free.

1. `lvs vg-guests`: data percent and metadata percent. Metadata full is
   worse (writes fail across every volume): `lvextend --poolmetadatasize`.
2. Extend the data disk in Azure (`tofu apply` with a larger
   `data_disk_gb`, online for Premium SSD v2), then `pvresize` and
   `lvextend -l +100%FREE vg-guests/thin`.
3. Find who: `factory-admin projects list --host host-NN --sort disk`.
   Over-allocated thin volumes are fine; used space is what matters.

## StoreFull

Host root filesystem over 85 percent, almost always `/nix/store`.

1. `nix path-info -S --all | sort -k2 -n | tail`: biggest closures.
   hostd keeps a GC root per live guest closure; everything else is
   collectable.
2. `nix-collect-garbage` (hostd runs it weekly; run it now). If still
   full, a tenant's fragment pulled something huge: `factory-admin
   projects list --host host-NN --sort closure` and talk to them, or
   lower the closure cap.

## GuestdLost

hostd cannot talk to a guest's guestd for 5 minutes.

1. `factory-admin projects show <id>`: is the guest `running`? If the
   guest crashed, the transient unit is gone: `factory-admin projects
   start <id>`.
2. If running: the guest's console log at
   `/var/lib/factory/guests/<id>/console.log`. An OOM in the guest (the
   tenant filled memory) usually killed guestd; the kernel line names it.
   `factory-admin projects restart <id>` (stop without snapshot, since
   freeze needs guestd, then start).
3. Sampling for that guest is missing for the window; billing uses the
   last known state, so a running guest is still billed.

## RollupLag

The hourly usage rollup is more than 2 hours behind.

1. `journalctl` on the api container for `rollup_` events. A failing hour
   is retried; a poisoned row (a sample with a negative delta from a
   hostd restart) is skipped and logged with the project id.
2. `factory-admin billing rollup --hour <hour>` to re-run one hour.

## StripePushFail

Usage records failed to push.

1. Stripe dashboard, API logs. A 400 usually means the subscription item
   id changed (a user changed plan): `factory-admin billing resync
   --user`.
2. Rows keep `stripe_usage_record_id = null` and are retried hourly; no
   usage is lost.

## Host never registered

A new host has been up for more than five minutes and is not in `hosts
list`.

1. `ssh root@<private ip>` via the edge: `journalctl -u hostd`. `token_expired`
   or `token_used`: mint a new one (`factory-admin hosts add --reissue`),
   write it to `/run/factory/join-token`, `systemctl restart hostd`.
2. `kvm_missing`: the VM was created without `security_type = Standard`.
   Destroy and recreate; there is no in-place fix.
3. `pool_missing`: the data disk is not attached or `disko` did not run.
   `lsblk`; re-run nixos-anywhere if the layout is missing.

## Guest unresponsive

A tenant reports `factory attach` hangs, or `Freeze` times out.

1. `factory-admin projects show`: state and last signals. `guestd_ok=false`
   means "GuestdLost" above.
2. `factory-admin exec <id> -- uptime` (goes through vsock; if it works,
   the guest is fine and the problem is the gateway or the tenant's
   certificate).
3. Console log for kernel panics or OOM. A panic leaves CH running with a
   dead guest: `factory-admin projects restart`.

## Reservation drift

`hosts list` free memory does not match the sum of running guests.

`factory-admin hosts reconcile host-NN` asks hostd for its `Hello` state
and recomputes reservations. Happens after an api crash mid-command; the
reconcile is safe to run any time.

## Coolify deploy failed

The api or web app's rolling deploy did not go green.

1. Coolify's deployment log. A health check timeout with the container
   alive is usually a migration running long (the api runs migrations at
   start): wait, the old container is still serving.
2. A migration failure leaves the new container crash-looping and the old
   one serving. Fix forward or `factory-admin db rollback --to <previous>`
   from the operator machine (it connects to Postgres over the Coolify
   VM's WireGuard address), then redeploy the previous image tag.
3. If the deploy fell back to stop-then-start (Coolify does this silently
   when the health check is missing), the health check config was lost;
   restore it before the next deploy.

## Postgres restore

1. Provision a scratch Coolify (or use staging), add a Postgres database,
   download the newest dump from R2 (`rclone ls r2:factory-pg-backups`),
   `pg_restore` into it.
2. Point a staging api at it, run `factory-admin db verify` (row counts
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

1. `factory-admin hosts mark-lost host-NN`: every project on it goes to
   `error` with reason `host_lost`, tenants get an email.
2. For each project: `factory-admin projects restore <id> --latest --to
   <other host>`. Data since the last snapshot (up to 24 hours, or since
   the last `stop`) is lost; the email says so. Tenants' git remotes hold
   whatever their agents pushed.
3. `factory-admin hosts retire host-NN`; `tofu apply` with it removed.

## Suspected cross-tenant access

1. `factory-admin hosts drain --all` (no new placements anywhere).
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

`factory-admin users suspend <handle> --reason "..."`: stops all their
guests (with snapshot), sets `billing_status = suspended`, revokes their
certificates, and writes an audit row. Their data is retained on the normal
30-day schedule from the moment of suspension unless `--retain` is passed.
