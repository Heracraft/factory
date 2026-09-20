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
4. **Postgres, as a file.** Project -> Add resource -> Services -> Docker
   Compose Empty, paste `ops/coolify/postgres/docker-compose.yml`, server
   the one just added (`DECISIONS.md` I-87). A Service rather than a git
   application because Coolify gives the postgres image inside a Service
   its Backups tab. Leave "Connect to predefined network" off: the file
   joins the shared `coolify` network itself, which registers the service
   name `repose-postgres` there for the applications (fact 11, I-89).
   `POSTGRES_PASSWORD`
   is a project-level shared variable so the api applications reference
   the same one. Then the backup destination, below.
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
   directory `/`, each with its env file from `ops/coolify/` pasted in.
   Domains
   `https://api.repose.herakraft.co:8080` and
   `https://repose.herakraft.co:3000`; port mappings and health-check
   settings are in `ops/coolify/README.md`. Secrets — Logto M2M, the Entra
   client, later Stripe and Resend — go in each app's Environment tab,
   nowhere else. No pre-deploy command on either: the api applies its own
   migrations at start and generates the platform CA the first time it
   finds none, both idempotent (fact 12, I-90). `api-grpc` deploys after
   `api`.
7. **Deploy**, then the WireGuard peer to the edge: `infra/README.md`,
   "Wiring the control plane to the edge". Two moves, because the private
   key never leaves the VM.

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

That output is the form Coolify asks for under S3 Storages, field by field.
Two of them are got wrong reliably: the **endpoint carries its scheme**
(`https://<account id>.r2.cloudflarestorage.com`) and the **region is the
literal `auto`**, not blank and not an AWS region.

Then, on the Postgres service, Backups: nightly at 02:00, retention 35
days, destination the S3 storage just added. Coolify's retention and the
bucket's lifecycle rule are both set to 35 days on purpose — whichever one
is misconfigured later, the other still bounds the bill and the exposure.

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

## Coolify facts that cost a round trip each (2026-09-20)

Verified in Coolify's source (`bootstrap/helpers/parsers.php`) or against
the live instance, not taken from the docs. Each one changed a file here.

1. **`container_name` in a compose file is overwritten** with
   `<service>-<uuid>`, and `networks: aliases:` are dropped: the parser
   `<service>-<uuid>`. `networks: aliases:` are **not** dropped for a
   service: the parser's "ignore aliases" applies to top-level network
   definitions, and a service's own `networks:` map is passed through
   (serviceParser, 4.3.23). Docker registers the service name on every
   network the file joins, so `repose-postgres` (not `postgres`, which a
   second such service on the shared network would round-robin with) is
   the hostname.
2. **Compose resources get their own network; applications sit on the
   server's `coolify` network.** A compose Service reaches the applications,
   and they reach it, only with **Connect to predefined network** on for
   the Service (Configuration -> Advanced). The applications need no
   toggle. An open Coolify issue reports the toggle sometimes not attaching
   the network for Services: `docker inspect` the container for the
   `coolify` network before debugging anything else (`RUNBOOK.md`
   "api cannot resolve repose-postgres").
3. **The Backups tab exists for a postgres image inside a Service**
   (pasted compose), not for a compose added as a git application. That is
   why `ops/coolify/postgres/docker-compose.yml` is pasted rather than
   pulled from the repository; the file in the repository stays the source
   of truth, and a change to it is a paste.
4. **Coolify's own health check runs curl or wget inside the container.**
   The api image is distroless and has neither, so it carries
   `HEALTHCHECK CMD api -healthcheck` and Coolify's check is turned off on
   `api` and `api-grpc`; with no health signal Coolify silently falls back
   to stop-then-start, which is the downtime the rolling deploy exists to
   avoid.
5. **One resource per thing.** A compose resource redeploys as a unit and
   is not a rolling deploy. Postgres has nothing to roll, so it is the one
   compose file; `api`, `api-grpc` and `web` are three Dockerfile
   applications, each redeployed only for its own change (I-87).
6. **Coolify does what Coolify does.** Backups, health checks, the proxy,
   TLS: configured, not reimplemented. Sidecars that duplicated the backup
   were written and removed the same day (I-87).
7. **Coolify SSHes to a server from inside its own container.** The VM
   answering `tailscale ping` from the Coolify host proves the tailnet, not
   that the `coolify` container can route to it; sshd's journal on the VM
   (`journalctl -u ssh`) shows whether any connection arrived at all.
   `docker exec coolify ssh root@<tailnet ip> true` on the Coolify host is
   the test that matches what Coolify does.
8. **Magic variables and required values.** `${VAR:?}` in a compose file
   makes Coolify refuse to deploy until the value is set, which is how the
   file declares its secrets without holding them; `SERVICE_*` names are
   generated by Coolify. A project-level shared variable
   (`{{project.NAME}}`) is how four resources agree on one password.
9. **A shared-variable reference resolves only as a whole value.**
   `DATABASE_URL=postgres://repose:{{project.POSTGRES_PASSWORD}}@...` reached
   the container with the braces intact (2026-09-20): Coolify's environment
   variable model looks a reference up only when the value starts with
   `{{` and ends with `}}`, never inside a longer string. So the password is
   its own variable, `PGPASSWORD={{project.POSTGRES_PASSWORD}}`, and
   `DATABASE_URL` carries no password; pgx reads `PGPASSWORD` (libpq's
   convention) when the URL omits one, verified against pgx v5.
10. **DNS validation compares a domain with the server's address as
   entered, not with where the proxy listens.** The server is registered by
   its tailnet IP (I-86), so Coolify refused `api.repose.herakraft.co`
   because the record points at the public IP (2026-09-20). The check has
   no notion of a management address distinct from a served one; turn it
   off under Settings -> Advanced, "DNS validation". The records stay as
   `infra/README.md` "DNS" lists them.
11. **"Connect to predefined network" registers only the container name.**
   The toggle does not add `coolify` to the generated compose (its
   `networks:` lists only the per-resource network,
   `/data/coolify/services/<uuid>/docker-compose.yml` on the VM); Coolify
   runs `docker network connect` afterwards, and Docker registers the
   container name there and nothing else, so the first api deploy failed
   with `repose-postgres` NXDOMAIN (2026-09-20). The fix is in the file:
   the service lists `coolify` (declared `external: true`) under its own
   `networks:`, and Docker then registers the service name and any
   `aliases` there. Verified on the VM with a throwaway compose: aliases on
   `coolify` came out as the container name, `repose-postgres` and the
   explicit alias, and a busybox reached 5432 by name. The toggle stays
   off for this Service.
12. **A pre-deployment command runs in the previous container, or not at
   all.** `ApplicationDeploymentJob::run_pre_deployment_command` (4.3.23)
   does `docker exec` into a currently running container of the app and
   logs "No running containers found. Skipping." when there is none. So
   `repose-admin db migrate` as a pre-deploy command never ran on the first
   deploy, and on later ones would have run the old image's migrations;
   `repose-admin ca init` "after the first deploy" had no container to run
   in, because an api that exits on a missing CA never goes healthy and
   Coolify removes it. The api now does both itself at start (I-90).

13. **A rolling deploy still drops a request or two at the switchover.**
   Measured on `web`, 2026-09-20: two loops at five requests a second
   against `https://repose.herakraft.co/` and `/healthz` saw ~10,400
   responses and exactly one failure each, both at 18:05:15Z, eleven
   seconds after the new container started (created 18:05:02.851Z,
   started 18:05:04.014Z, image `e6dd4fa`). The failures were 5-second
   *hangs*, not 502s, which points at Traefik keeping the outgoing
   container in its pool for a moment after Coolify removes it rather
   than at the health check — the new container was healthy before the
   old one went. So "rolling" here means "no outage", not "no dropped
   request": a user reloading at that instant waits five seconds. It is
   worth knowing before it is measured on `api`, where the same gap is a
   CLI command failing rather than a page taking a moment.
   `ops/deploy-probe.sh` is the loop that measures it.

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

`ops/restore-rehearsal.sh` is that procedure as one command with a clock
on it:

```bash
scp ops/restore-rehearsal.sh root@<control ip>:/root/
ssh root@<control ip> /root/restore-rehearsal.sh
```

It finds the newest object in the bucket, restores it into a throwaway
`postgres:16-alpine` container of its own — never into `repose-postgres`,
and with no published port — runs `repose-admin db verify` from the api
image against it, prints fetch, restore and verify times, and removes the
container on the way out (and on Ctrl-C). It reads both shapes of dump,
gzipped plain SQL and custom format, by looking at the bytes rather than
at the name, because which one Coolify writes depends on how the backup
was set up. Run it after any schema change that moves a lot of rows: the
number it prints is what an incident will cost, and a number from before
the data grew is not that number.

Rehearsing onto a scratch *server* added to the same Coolify (staging's
control VM, `coolify_count = 1` in `staging.tfvars`) is still the fuller
exercise, because it also proves the Coolify half; the script is the part
that has to work under pressure, and it can be run on the control VM
itself without risk.

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
