# The control plane on Coolify

`docs/workstreams/11-infra-opentofu.md` §3 draws a line: OpenTofu creates the
VM, and Coolify keeps its application definitions in its own database, so
everything on the far side of that line is a click path. This file is that
click path, in the order it works, plus the two things about this Coolify that
are not obvious and will ruin a restore if they are learned late.

The VM itself is `infra/azure/modules/coolify`: an Ubuntu 24.04
`Standard_D4s_v7` with a 256 GB Premium OS disk and a static public IP, in the
`control` subnet, whose NSG opens 80, 443 and 22 to `control_web_cidrs` and
`operator_cidrs` and nothing else. It stays Ubuntu because Coolify rejects
NixOS as a host or a managed server (`DECISIONS.md` R4-2).

## Why there is no port 8000 in the NSG

Coolify's dashboard listens on **8000, over plain HTTP**, and until an admin
account exists anyone who reaches it can create one. The control subnet NSG
deliberately has no rule for it, and `infra/azure/modules/network/main.tf`
carries a `postcondition` that fails the plan if somebody adds one "just for
setup". The way in is the port that is already open:

```bash
ssh -N -L 8000:127.0.0.1:8000 root@$(tofu -chdir=infra/azure/prod output -raw control_public_ip)
# then open http://127.0.0.1:8000
```

The same tunnel is the right way to reach it afterwards. Coolify's own
dashboard never needs to be on the internet; `repose.herakraft.co`,
`api.repose.herakraft.co` and `auth.repose.herakraft.co` are served by the
Traefik it manages, on 80 and 443, which are the ports the NSG does open.

## First boot, in order

The apply does not return until Coolify is healthy: `terraform_data.ready`
waits for cloud-init, then reads the `coolify` container's Docker health
status, which is the same signal the installer itself waits on. If the apply
fails there, `docker logs coolify` on the VM is the next line, and
`/var/log/cloud-init-output.log` is the one before it.

1. **Tunnel in** (above) and create the admin user. It is created by hand
   rather than by cloud-init's `ROOT_USER_PASSWORD`, because `custom_data`
   stays in the Azure VM model and is readable through IMDS for the life of
   the VM — the same reason the join token does not travel that way
   (`DECISIONS.md` I-20).
2. **Add the server as `localhost`.** Coolify deploys to itself.
3. **Back up `/data/coolify/source/.env` off the machine, now.** See
   "The .env is half the backup" below. This is the step that is skipped and
   then wanted.
4. **Postgres**: add the platform database as a Coolify-managed Postgres.
   Then the backup destination, below, then `repose-admin db migrate`.
5. **Logto**: a Docker Image resource, `ghcr.io/logto-io/logto`, with
   `ENDPOINT` and `ADMIN_ENDPOINT` at `auth.repose.herakraft.co`. Configure
   the GitHub connector, the API resource
   `https://api.repose.herakraft.co`, and two applications: `repose-cli`
   (Native, device flow on) and `repose-web` (SPA).
6. **api and web**: Dockerfile applications from the repository, health checks
   `/healthz` and `/`, environment from `ops/coolify/api.env.example`. The
   gRPC listener is a second app from the same image with `API_MODE=grpc`
   (`DECISIONS.md` I-2). Secrets — CA keys, the Key Vault client certificate,
   Stripe and Resend keys — go in Coolify's secret store, never in the env
   file.
7. **WireGuard to the edge**: `infra/README.md`, "Wiring the control plane to
   the edge". Two moves, because the private key never leaves the VM.

A health check on every application is not optional: without one Coolify
silently falls back to stop-then-start instead of a rolling deploy, and the
first anybody hears of it is downtime during a routine deploy
(`RUNBOOK.md` "Coolify deploy failed").

## The Postgres backup to R2

The bucket, its 35-day lifecycle rule and its incomplete-upload cleanup are
`infra/r2`. The API token is not: a token created by OpenTofu would sit in the
state file in clear text for the life of the bucket, so it is a human step
next to the other credentials (`DECISIONS.md` I-21, `AZURE-SETUP.md` step 10).

```bash
export CLOUDFLARE_API_TOKEN=...          # R2 object read/write on the bucket
make -C infra plan ENV=r2 && make -C infra apply ENV=r2
tofu -chdir=infra/r2 output coolify_s3_destination
```

That output is the form Coolify asks for, field by field. Two of them are got
wrong reliably: the **endpoint carries its scheme**
(`https://<account id>.r2.cloudflarestorage.com`) and the **region is the
literal `auto`**, not blank and not an AWS region.

Then, on the Postgres resource: backups on, nightly at 02:00, retention 35
days, destination the S3 storage just added. Coolify's retention and the
bucket's lifecycle rule are both set to 35 days on purpose — whichever one is
misconfigured later, the other still bounds the bill and the exposure.

Run one backup by hand from the UI before trusting the schedule, then:

```bash
ssh root@<control ip> repose-backup-check
# repose-backup-check: newest dump is 0h old, written 2026-09-20T02:00:11Z
```

`repose-backup-check` is installed by cloud-init and needs an rclone remote
named `r2`, created once from the same token (the command is in the script's
own header, and in the `rclone_hint` field of the tofu output). It exits
non-zero when the newest object is older than 36 hours, which is a nightly
dump plus one missed night. `RUNBOOK.md` "PostgresBackupStale" is the entry
that calls it.

## The .env is half the backup

Coolify encrypts the credentials it holds — every application's secrets, the
database passwords it generated — with `APP_KEY` from
`/data/coolify/source/.env`. **A Postgres dump restored without that file is a
database of ciphertext nobody can read.** The dump in R2 is therefore not a
complete backup of the control plane on its own.

Keep `/data/coolify/source/.env` in the password manager, and update it after
any change that rewrites it. The installer says so too, in the last line
before its banner, which is exactly where nobody reads it.

## Restore rehearsal

The release checklist wants this timed at least once
(`docs/CHECKLIST.md`, "Postgres restore from R2 rehearsed"). The procedure is
`RUNBOOK.md` "Postgres restore"; `rclone` and `pg_restore` are installed on
the control-plane VM by cloud-init so that it can be followed without stopping
to install anything, and the readiness provisioner fails the apply if either
is missing.

Rehearse onto staging or a scratch Coolify, never onto production, and start
from the `.env` as well as the dump — a rehearsal that reuses the running
machine's `APP_KEY` proves nothing about a real restore.

## Upgrading Coolify

`coolify_version` is pinned (`4.3.23` at the time of writing) and
`AUTOUPDATE=false`, because an unattended upgrade of the thing that deploys
the api is a deploy nobody reviewed. Upgrading is deliberate:

1. Read the release notes; check `https://cdn.coollabs.io/coolify/versions.json`
   for what `v4` currently points at.
2. Back up `/data/coolify/source/.env` and take a manual Postgres backup.
3. Upgrade from the dashboard, or re-run the installer with the new version.
4. Raise `coolify_version` in `prod.tfvars` in the same change, so a rebuilt
   VM lands on the version that is actually running. The VM ignores
   `custom_data` changes, so this edit alone never touches the running
   machine — which is the point: the variable records reality, the upgrade
   performs it.

## What is still a human step

- The admin account (above).
- The R2 API token (`DECISIONS.md` I-21).
- The api's Entra app registration and its client certificate, whose object id
  becomes `api_identity_object_id` and turns on the Key Vault wrap/unwrap
  policy (`DECISIONS.md` I-21).
- Every click path in this file. Coolify's API could automate some of it; that
  is not built, and `DECISIONS.md` R3-1 keeps provisioning in OpenTofu, which
  cannot reach into Coolify's database.
