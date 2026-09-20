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
Docker's apt repository; Tailscale installed but not joined; `rclone` and
`pg_restore` for the runbook's restore procedure; the WireGuard peer script;
and nothing listening but sshd. `terraform_data.ready` fails the apply if any
of that is missing.

## Adding the server, in order

1. **Join the tailnet.** Coolify reaches the VM over Tailscale, so no
   address of the owner's server goes in any file and nothing breaks when
   that server moves (`DECISIONS.md` I-86). Once, over SSH:

   ```bash
   ssh root@<control ip> tailscale up          # prints a login URL; approve it
   ssh root@<control ip> tailscale ip -4       # the address Coolify will use
   ```

   A tagged node (`--advertise-tags=tag:repose`) keeps the machine out of a
   personal account; the tag must exist in the tailnet ACL first. Without
   Tailscale, the fallback is `coolify_manager_cidrs` in `prod.local.tfvars`
   (the instance's public `/32`) and an apply; that rule goes stale when the
   owner's server moves, and the failure is a silent hang in step 2.
2. **Add the server** in Coolify: Servers, Add. Name it after the VM
   (`coolify-01`), address the tailnet IP from step 1, user `root`, port
   `22`, and pick the private key whose public half is `coolify_public_key`.
   Then **Validate & configure**. Coolify checks SSH, finds Docker already
   installed, writes its `/etc/docker/daemon.json`, and starts its proxy
   (Traefik) on 80 and 443. A validation that fails on "permission denied"
   is the key: compare `Keys & Tokens` with `prod.tfvars`. One that hangs is
   the network: `tailscale status` on the VM, or the NSG if using the
   fallback.
3. **Names and proxy.** The proxy on this server is what serves
   `repose.herakraft.co` and `api.repose.herakraft.co`; both A records point
   at the VM's public IP (`infra/README.md`, "DNS"). 80 and 443 are open to
   the internet (`control_web_cidrs = ["0.0.0.0/0"]`, owner's call
   2026-09-20), so Let's Encrypt HTTP-01 works as it does everywhere.
4. **The control plane, as one file.** Project -> Add resource -> Docker
   Compose from git: repository `heracraft/repose`, branch `main`, base
   directory `/`, compose file `/ops/coolify/docker-compose.yml`, server
   the one just added. `ops/coolify/README.md` lists the values Coolify
   asks for and the two Domains fields
   (`https://api.repose.herakraft.co:8080` on `api`,
   `https://repose.herakraft.co:3000` on `web`). The file carries Postgres,
   the two apis, the dashboard and the backup pair (`DECISIONS.md` I-87);
   the deploy refuses until every `${VAR:?}` has a value.
5. **Logto** is the owner's existing instance, `https://accounts.herakraft.co`
   (`DECISIONS.md` I-84); nothing is deployed for it. In that Logto: the API
   resource `https://api.repose.herakraft.co`, two applications,
   `repose-cli` (Native, device flow on, redirect
   `http://127.0.0.1:*/callback`) and `repose-web` (SPA, redirect
   `https://repose.herakraft.co/callback`), and the M2M application
   `repose-api` with a role granting the Management API, whose id and
   secret are `LOGTO_M2M_CLIENT_ID/SECRET`. The api and CLI take the
   endpoint without `/oidc` and append it.
6. **api, api-grpc, web**: three Coolify "Dockerfile" applications from the
   repository (`cmd/api/Dockerfile` twice, `apps/web/Dockerfile`), base
   directory `/`, each with its env file from `ops/coolify/` pasted in and
   "Connect to predefined network" on. Domains
   `https://api.repose.herakraft.co:8080` and
   `https://repose.herakraft.co:3000`; port mappings and health-check
   settings are in `ops/coolify/README.md`. Secrets — Logto M2M, the Entra
   client, later Stripe and Resend — go in each app's Environment tab,
   nowhere else. `api` runs `repose-admin db migrate` as its pre-deploy
   command; `api-grpc` deploys after it.
7. **Deploy**, then `repose-admin ca init` once against the database
   (`RUNBOOK.md` "Control plane"), and the WireGuard peer to the edge:
   `infra/README.md`, "Wiring the control plane to the edge". Two moves,
   because the private key never leaves the VM.

A health check on every application is not optional: without one Coolify
silently falls back to stop-then-start instead of a rolling deploy, and the
first anybody hears of it is downtime during a routine deploy
(`RUNBOOK.md` "Coolify deploy failed"). On `api` and `api-grpc` the check
is the image's own `HEALTHCHECK` (`api -healthcheck`), because Coolify's
curl-based one cannot run in a distroless image; Coolify's is turned off
there so the image's drives the deploy (I-87).

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

The token's access key id, secret and the endpoint (with its `https://`)
are `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY` and `R2_ENDPOINT` on the
Postgres compose resource. The backup itself is two services in that file
(I-87):
`pg-backup` dumps nightly at `BACKUP_HOUR_UTC` (02:00) into a volume and
prunes after 35 days; `backup-sync` copies new dumps to the bucket every 15
minutes and never deletes there, so the bucket's lifecycle rule and the
local prune are both set to 35 days on purpose — whichever one is
misconfigured later, the other still bounds the bill and the exposure.

Run one by hand before trusting the schedule (Coolify's terminal on
`pg-backup`: `pg-backup once`; the sync follows within 15 minutes), then:

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

- Joining the tailnet and adding the server (above).
- The Logto applications, the `repose-api` M2M application and the API
  resource in the owner's Logto (above).
- The values under each resource's Environment tab and the two Domains
  fields (`ops/coolify/README.md`).
- The R2 API token (`DECISIONS.md` I-21).
- The api's Entra app registration and its client certificate, whose object id
  becomes `api_identity_object_id` and turns on the Key Vault wrap/unwrap
  policy (`DECISIONS.md` I-21).
- Every click path in this file. Coolify's API could automate some of it; that
  is not built, and `DECISIONS.md` R3-1 keeps provisioning in OpenTofu, which
  cannot reach into Coolify's database.
