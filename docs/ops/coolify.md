# The control plane on Coolify

`docs/workstreams/11-infra-opentofu.md` §3 draws a line: OpenTofu creates the
VM, and Coolify keeps its application definitions in its own database, so
everything on the far side of that line is a click path. This file is that
click path, in the order it works.

The Coolify is the one the owner already runs on their personal server. The
control VM is a **server** of that instance (`DECISIONS.md` I-83): Coolify
connects to it over SSH as root, installs its proxy, and deploys the api, the
dashboard, Logto and the platform Postgres onto it. Nothing Coolify runs on
the VM itself, so there is no admin account to create there, no port 8000, no
`APP_KEY` to copy off it, and no Coolify upgrade to schedule for it.

The VM itself is `infra/azure/modules/coolify`: an Ubuntu 24.04
`Standard_D4s_v7` with a 256 GB Premium OS disk and a static public IP, in the
`control` subnet, whose NSG opens 80 and 443 to `control_web_cidrs`, and 22 to
`operator_cidrs` plus `coolify_manager_cidrs` (the instance's address), and
nothing else. It stays Ubuntu because Coolify rejects NixOS as a managed
server (`DECISIONS.md` R4-2). cloud-init leaves it exactly as Coolify's
server validation wants to find it: root login by key, with the operator keys
and the instance's key (`coolify_public_key` in `prod.tfvars`) in
`/root/.ssh/authorized_keys`; Docker Engine and the compose plugin from
Docker's apt repository; `rclone` and `pg_restore` for the runbook's restore
procedure; the WireGuard peer script; and nothing listening but sshd.
`terraform_data.ready` fails the apply if any of that is missing.

## Adding the server, in order

1. **Open the NSG to the instance.** `coolify_manager_cidrs` in
   `prod.local.tfvars` is the public address the owner's Coolify connects
   from, as a `/32`. Apply. Without it "Validate & configure" times out and
   looks like a dead machine.
2. **Add the server** in Coolify: Servers, Add. Name it after the VM
   (`coolify-01`), address `tofu -chdir=infra/azure/prod output
   control_public_ip`, user `root`, port `22`, and pick the private key
   whose public half is `coolify_public_key`. Then **Validate & configure**.
   Coolify checks SSH, finds Docker already installed, writes its
   `/etc/docker/daemon.json`, and starts its proxy (Traefik) on 80 and 443.
   A validation that fails on "permission denied" is the key: compare
   `Keys & Tokens` with `prod.tfvars`. One that hangs is the NSG (step 1).
3. **Wildcard and proxy.** The proxy on this server is what serves
   `repose.herakraft.co`, `api.repose.herakraft.co` and
   `auth.repose.herakraft.co`; the DNS A records for all three point at the
   VM's public IP (`infra/README.md`, "DNS"). Pre-launch, 80 and 443 are open
   to `control_web_cidrs` only, so Let's Encrypt HTTP-01 cannot reach the
   proxy: either add the instance's own address and Let's Encrypt to that
   list, or use DNS-01 on the proxy. At launch `control_web_cidrs` becomes
   `["0.0.0.0/0"]` and HTTP-01 works as it does everywhere else.
4. **Postgres**: on the new server, add the platform database as a
   Coolify-managed Postgres. Then the backup destination (below), then
   `repose-admin db migrate`.
5. **Logto**: a Docker Image resource on the server, `ghcr.io/logto-io/logto`,
   with `ENDPOINT` and `ADMIN_ENDPOINT` at `auth.repose.herakraft.co`.
   Configure the GitHub connector, the API resource
   `https://api.repose.herakraft.co`, and two applications: `repose-cli`
   (Native, device flow on) and `repose-web` (SPA). If the owner's existing
   Logto is preferred over a second one, the api only needs its issuer and
   JWKS URL (`ops/coolify/api.env.example`); the two applications and the
   API resource are the same either way.
6. **api and web**: Dockerfile applications from the repository, on the
   server, health checks `/healthz` and `/`, environment from
   `ops/coolify/api.env.example`. The gRPC listener is a second app from the
   same image with `API_MODE=grpc` (`DECISIONS.md` I-2). Secrets — CA keys,
   the Key Vault client certificate, Stripe and Resend keys — go in
   Coolify's secret store, never in the env file.
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

`repose-backup-check` is installed on the VM by cloud-init and needs an
rclone remote named `r2`, created once from the same token (the command is in
the script's own header, and in the `rclone_hint` field of the tofu output).
It exits non-zero when the newest object is older than 36 hours, which is a
nightly dump plus one missed night. `RUNBOOK.md` "PostgresBackupStale" is the
entry that calls it.

## The instance's .env is half the backup

Coolify encrypts the credentials it holds — every application's secrets, the
database passwords it generated — with `APP_KEY` from
`/data/coolify/source/.env` **on the owner's Coolify host**, not on this VM.
**A Postgres dump restored into a Coolify without that key is a database of
ciphertext nobody can read.** The dump in R2 is therefore not a complete
backup of the control plane on its own; the owner's existing backup of their
Coolify host is the other half, and it was already their problem before this
project. Check that it exists.

## Restore rehearsal

The release checklist wants this timed at least once
(`docs/CHECKLIST.md`, "Postgres restore from R2 rehearsed"). The procedure is
`RUNBOOK.md` "Postgres restore"; `rclone` and `pg_restore` are installed on
the control VM by cloud-init so that it can be followed without stopping to
install anything, and the readiness provisioner fails the apply if either is
missing.

Rehearse onto a scratch server added to the same Coolify (staging's control
VM, `coolify_count = 1` in `staging.tfvars`), never onto production.

## Upgrading Coolify

Not a step here. The instance is the owner's; its upgrade routine is
whatever it already was. Nothing the api, the dashboard or Logto rely on is
specific to a Coolify version (`DESIGN.md` §Risks).

## What is still a human step

- Adding the server, and `coolify_manager_cidrs` (above).
- The R2 API token (`DECISIONS.md` I-21).
- The api's Entra app registration and its client certificate, whose object id
  becomes `api_identity_object_id` and turns on the Key Vault wrap/unwrap
  policy (`DECISIONS.md` I-21).
- Every click path in this file. Coolify's API could automate some of it; that
  is not built, and `DECISIONS.md` R3-1 keeps provisioning in OpenTofu, which
  cannot reach into Coolify's database.
