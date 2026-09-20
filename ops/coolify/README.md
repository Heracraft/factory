# The control plane on Coolify, as files

Four Coolify resources on the control VM, one per thing, so each deploys on
its own and the three applications get rolling deploys (DECISIONS I-87):

| resource | type | from | env |
| --- | --- | --- | --- |
| `repose-postgres` | Docker Compose | `postgres/docker-compose.yml` | its `${VAR:?}` values in the resource's Environment tab |
| `api` | Dockerfile application, `cmd/api/Dockerfile`, context `/` | this repo, `main` | `api.env.example` |
| `api-grpc` | Dockerfile application, same Dockerfile | this repo, `main` | `api-grpc.env.example` |
| `web` | Dockerfile application, `apps/web/Dockerfile`, context `/` | this repo, `main` | `web.env.example` |

`docs/ops/coolify.md` is the order of operations around them.

## Postgres

The compose resource is Postgres plus its backup: `pg-backup` dumps nightly
(`pg_dump -Fc`) into a volume and prunes after 35 days; `backup-sync`
copies new dumps to R2 every 15 minutes and never deletes there. The
container is named `repose-postgres` and joins the shared `coolify` network
so the api applications reach it by that name. Values: `POSTGRES_PASSWORD`
(a project-level shared variable, so the api apps reference the same one
as `{{project.POSTGRES_PASSWORD}}`), `R2_ACCESS_KEY_ID`,
`R2_SECRET_ACCESS_KEY`, `R2_ENDPOINT` (from the R2 API token, DECISIONS
I-21; the endpoint with its `https://`). Optional: `BACKUP_HOUR_UTC` (2),
`BACKUP_KEEP_DAYS` (35), `R2_BUCKET`.

## The three applications

Each is Coolify "Dockerfile" build pack from this repository with base
directory `/` and the Dockerfile path above. Paste the matching env file
into the app's Environment (developer view) and fill the blanks. On every
app: **Connect to predefined network** on (that is how they reach
`repose-postgres`).

- `api`: domain `https://api.repose.herakraft.co:8080`; port mapping
  `9103:9103` (metrics, over WireGuard only); pre-deploy command
  `repose-admin db migrate`; Coolify's own health check **off** so the
  image's `HEALTHCHECK` (`api -healthcheck`) drives the rolling deploy (the
  image is distroless, so Coolify's curl-based check cannot run in it).
- `api-grpc`: no domain; port mappings `8443:8443`, `8444:8444`,
  `9104:9103`; Coolify's health check **off** (same reason); deploy after
  `api`. The control NSG never opens these ports; hosts and the edge use
  the VM's WireGuard address `10.255.255.1`, and hostd is registered with
  `apiServerName = api.repose.herakraft.co`, the first of
  `GRPC_SERVER_NAMES`, for which the gRPC certificate is issued from the
  api's CA at start.
- `web`: domain `https://repose.herakraft.co:3000`; health check path
  `/healthz`, port 3000.

Required before anything serves: `LOGTO_M2M_CLIENT_ID/SECRET` (the
`repose-api` M2M application in Logto, Management API role),
`PUBLIC_LOGTO_APP_ID` (the `repose-web` SPA), `AZURE_TENANT_ID`,
`AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET` (the api's Entra app registration,
`docs/ops/AZURE-SETUP.md`). Empty until turned on: `RESEND_API_KEY`,
`STRIPE_*`, `PUBLIC_STRIPE_PUBLISHABLE_KEY`, `OTEL_EXPORTER_OTLP_ENDPOINT`.

## Backups

Verify with `ssh root@<control ip> repose-backup-check` (needs the `r2`
rclone remote on the VM, once, from the same token). A manual dump: in
Coolify's terminal for `pg-backup`, `pg-backup once`; the sync follows
within 15 minutes. Restore: `docs/ops/RUNBOOK.md` "Postgres restore".
