# The control plane on Coolify, as files

Four Coolify resources on the control VM, one per thing, so each deploys on
its own and the three applications get rolling deploys (DECISIONS I-87):

| resource | type | from | env |
| --- | --- | --- | --- |
| `repose-postgres` | Service (Docker Compose, pasted) | `postgres/docker-compose.yml` | `POSTGRES_PASSWORD`; backups in its Backups tab |
| `api` | Dockerfile application, `cmd/api/Dockerfile`, context `/` | this repo, `main` | `api.env.example` |
| `api-grpc` | Dockerfile application, same Dockerfile | this repo, `main` | `api-grpc.env.example` |
| `web` | Dockerfile application, `apps/web/Dockerfile`, context `/` | this repo, `main` | `web.env.example` |

`docs/ops/coolify.md` is the order of operations around them.

## Postgres

The compose file is Postgres alone. Add it as a Coolify **Service** (Add
resource -> Docker Compose Empty, paste the file): Coolify recognises the
postgres image inside a Service and gives it a Backups tab, where the
nightly dump to R2 is scheduled (`docs/ops/coolify.md`). Turn **Connect to
predefined network** **off** for it (Configuration -> Advanced): the file
joins the shared `coolify` network itself, so the service name
`repose-postgres` is registered there and is the `DATABASE_URL` host. The
toggle instead connects the container after `compose up`, which registers
only the container name (`docs/ops/coolify.md`, fact 11). Do not rename the
service. Its one value is `POSTGRES_PASSWORD`,
a project-level shared variable, so the api apps reference the same one as
`PGPASSWORD={{project.POSTGRES_PASSWORD}}`, on its own line: Coolify only
resolves a reference that is a variable's whole value, so the password is
not embedded in `DATABASE_URL`; pgx takes it from `PGPASSWORD`.

Coolify's DNS validation must be off (Settings -> Advanced): it compares each
domain with the server's tailnet address, not the public IP the proxy serves
on (`docs/ops/coolify.md`, fact 10).

## The three applications

Each is Coolify "Dockerfile" build pack from this repository with base
directory `/` and the Dockerfile path above. Paste the matching env file
into the app's Environment (developer view) and fill the blanks.
Applications sit on the server's `coolify` network by default, where
`repose-postgres` answers because the compose file joins that network; if
the `DATABASE_URL` host lookup fails, `docker inspect` the Postgres
container's aliases on `coolify` (`RUNBOOK.md` "api cannot resolve
repose-postgres").

- `api`: domain `https://api.repose.herakraft.co:8080`; port mapping
  `9103:9103` (metrics, over WireGuard only); **no pre-deploy command**
  (the api migrates itself at start and creates the CA on first start,
  I-90); Coolify's own health check **off** so the
  image's `HEALTHCHECK` (`api -healthcheck`) drives the rolling deploy (the
  image is distroless, so Coolify's curl-based check cannot run in it).
- `api-grpc`: no domain; port mappings `8443:8443`, `8444:8444`,
  `9104:9103` (Configuration -> Network, "Ports Mappings"; a Dockerfile
  application publishes nothing until they are set); Coolify's health check
  **off** (same reason); deploy after `api`. The control NSG never opens
  these ports on the public IP. Docker publishes them on every address, so
  they are reached on two private ones (DECISIONS I-92): hosts dial the VNet
  address (`control_private_ip`, `10.200.3.4`) because a host registers
  before it has a tunnel, and the edge dials the WireGuard address
  `10.255.255.1`. `GRPC_SERVER_NAMES` therefore lists all three names,
  `api.repose.herakraft.co,10.255.255.1,10.200.3.4` (the IPs become IP
  SANs), so the gateway and hostd verify the certificate the app issues
  from the api's CA at start without a server-name override; hosts also
  pass `apiServerName = api.repose.herakraft.co`.
- `web`: domain `https://repose.herakraft.co:3000`; health check path
  `/healthz`, port 3000.

Required before anything serves: `LOGTO_M2M_CLIENT_ID/SECRET` (the
`repose-api` M2M application in Logto, Management API role),
`PUBLIC_LOGTO_APP_ID` (the `repose-web` SPA), `AZURE_TENANT_ID`,
`AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET` (the api's Entra app registration,
`docs/ops/AZURE-SETUP.md`). Empty until turned on: `RESEND_API_KEY`,
`STRIPE_*`, `PUBLIC_STRIPE_PUBLISHABLE_KEY`, `OTEL_EXPORTER_OTLP_ENDPOINT`.

## Backups

Coolify's own: on the Postgres service, Backups, nightly at 02:00 to the R2
S3 storage, retention 35 days; the destination's fields are
`tofu -chdir=infra/r2 output coolify_s3_destination`. Verify with
`ssh root@<control ip> repose-backup-check` (needs the `r2` rclone remote
on the VM, once, from the same token). Restore: `docs/ops/RUNBOOK.md`
"Postgres restore".
