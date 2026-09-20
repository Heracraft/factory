# The control plane on Coolify, as files

Four Coolify resources on the control VM, one per thing, so each deploys on
its own and the three applications get rolling deploys (DECISIONS I-87):

| resource | type | from | env |
| --- | --- | --- | --- |
| `repose-postgres` | Service (Docker Compose, pasted) | `postgres/docker-compose.yml` | `POSTGRES_PASSWORD`; backups are the owner's, on its Backups tab |
| `api` | Dockerfile application, `cmd/api/Dockerfile`, context `/` | this repo, `main` | `api.env.example` |
| `api-grpc` | Dockerfile application, same Dockerfile | this repo, `main` | `api-grpc.env.example` |
| `web` | Dockerfile application, `apps/web/Dockerfile`, context `/` | this repo, `main` | `web.env.example` |

`docs/ops/coolify.md` is the order of operations around them.

## Postgres

The compose file is Postgres alone. Add it as a Coolify **Service** (Add
resource -> Docker Compose Empty, paste the file): Coolify recognises the
postgres image inside a Service and gives it a Backups tab, which is
where the owner sets the schedule and destination (I-112). Turn **Connect to
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

- `api`: domain `https://api.repose.herakraft.co:8080`; **no port
  mappings at all**, and see "The api's metrics" below for why its 9103
  is not published; **no pre-deploy command**
  (the api migrates itself at start and creates the CA on first start,
  I-90); Coolify's own health check **off** so the
  image's `HEALTHCHECK` (`api -healthcheck`) drives the rolling deploy (the
  image is distroless, so Coolify's curl-based check cannot run in it).
- `api-grpc`: no domain; port mappings `8443:8443`, `8444:8444`,
  `9104:9103` (Configuration -> Network, "Ports Mappings"; a Dockerfile
  application publishes nothing until they are set); Coolify's health check
  **off** (same reason). The control NSG never opens
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

## The api's metrics

`API_METRICS_LISTEN=:9103` serves 57 `repose_*` series inside the `api`
container, and nothing outside that container can reach them. This is the
one thing on this page that is **not** settled, so it is written down
rather than guessed at.

**What it cannot be: a port mapping.** An earlier version of this file
said `9103:9103` on `api`, by analogy with `api-grpc`. A published host
port means the old and the new container cannot both be up, so Coolify
cannot roll the app — it is exactly the constraint that made `api-grpc`
a separate application in the first place (DECISIONS I-2). `api-grpc`
can carry `9104:9103` precisely because it is the app that accepts
stop-then-start, hostd's reconnect absorbing the seconds. The HTTP api
is the one that must keep rolling, so it publishes nothing.

Two ways out, for the owner to choose between:

**A. A Traefik router on the api app.** Coolify passes custom labels
through to the container, so the proxy already in front of the api can
serve `/metrics` from the same container port without publishing it. The
label block, ready to paste into the app's Configuration -> Advanced ->
Custom labels:

```
traefik.enable=true
traefik.http.routers.api-metrics.rule=Host(`api.repose.herakraft.co`) && Path(`/metrics`)
traefik.http.routers.api-metrics.entryPoints=https
traefik.http.routers.api-metrics.tls=true
traefik.http.routers.api-metrics.tls.certresolver=letsencrypt
traefik.http.routers.api-metrics.service=api-metrics
traefik.http.routers.api-metrics.middlewares=api-metrics-allow
traefik.http.services.api-metrics.loadbalancer.server.port=9103
traefik.http.middlewares.api-metrics-allow.ipallowlist.sourcerange=10.255.0.0/16,10.200.0.0/16
```

Prometheus then scrapes `https://api.repose.herakraft.co/metrics`
resolved to the control VM's WireGuard address, and the allow-list keeps
the internet out even though the router is on the public entry point.
*For:* nothing new to run, rolling deploys untouched, and it rides the
certificate Traefik already has. *Against:* the metrics endpoint exists
on a public hostname and its privacy is one middleware line — a label
edited by hand in a UI, which is the class of thing this repository
otherwise keeps in files; and `ipallowlist` reads the source address
Traefik sees, so it has to be checked against what the proxy actually
observes over the tunnel rather than assumed.

**B. Grafana Alloy as a Coolify service.** A collector on the `coolify`
network scrapes `api:9103` and `api-grpc:9103` by container name — no
published port, no proxy, no allow-list — and remote-writes to the
owner's Prometheus. *For:* the scrape never leaves the docker network,
the deployment stays a file, and the same agent can replace Fluent Bit
on hosts and the edge and push logs and metrics through one path instead
of two (which would make `ops/prometheus/wireguard-peer.conf`'s whole
forwarding problem, DECISIONS I-94, go away for hosts as well).
*Against:* one more thing to run and upgrade, a second way of getting
metrics out alongside the Prometheus scrape the rest of `ops/` is built
around, and replacing Fluent Bit is a change to every host's
configuration that wants its own decision rather than riding along with
this one.

Until one is chosen, `ops/prometheus/prometheus.yml` scrapes `api-grpc`
on `10.255.255.1:9104` and the `api` target is commented out with this
section named, because a target that can never answer is an alert
nobody will thank you for.

## Backups

Coolify's, on the Postgres service's Backups tab, against a destination
the owner configures in their own Coolify. Nothing on this side is
involved: no bucket, no token, no check, no rehearsal script and no alert
(`docs/DECISIONS.md` I-112). A backup or restore question is answered in
Coolify.

One thing worth knowing before a restore rather than during one:
`docs/ops/coolify.md`, "The instance's .env is half any backup".
