# Launch prompts and model choice per workstream

## Model choice

| Workstream | Model | Why |
|---|---|---|
| 01 host-nixos, 02 guest-base, 12 nix-config-pipeline | Fable 5.1 | NixOS internals, microvm.nix, restricted eval: the deepest and least-documented work, and mistakes here are silent |
| 03 hostd | Fable 5.1 | systems code touching LVM, nftables, KVM, gRPC replay semantics; the tenant-safety core |
| 05 control-plane-api | Fable 5.1 | auth, CA, secrets, scheduler in one binary; a mistake is a security issue |
| 06 gateway-edge | Fable 5.1 | SSH protocol relay and certificate handling |
| 14 security | Fable 5.1 | adversarial review of everything above |
| 04 guestd | Opus 5 | well-specified, moderate |
| 09 billing | Opus 5 | money, but the shapes are conventional Stripe |
| 10 observability, 11 infra-opentofu | Opus 5 | broad but conventional |
| 07 cli, 08 dashboard, 13 notifications | Sonnet 5 | well-specified, user-facing, fast iteration |
| 00 benchmark (deferred) | Sonnet 5 | a procedure to follow |

If Fable is unavailable or rate-limited, Opus 5 takes its rows. Never
Haiku for building; it is fine for the `done-check` grep pass at the end.

Sequential sessions work the same way: one prompt per session, same
model choice, and `STATUS.md` carries the state between them.

## The short form: `/ws <nn>`

The repo ships a project skill at `.claude/skills/ws/SKILL.md`. In a worktree,
start Claude Code and type `/ws 03`; it loads the preamble and the `03`
block below and begins. `/ws m1`, `/ws m2`, `/ws m3` and `/ws m3-web` run
the integration sessions (the "M<n> bring-up" sections below). The full
text below is what the skill expands to, kept here so it can be read and
edited in one place.

## Launching agents in parallel

Parallel agents in one checkout collide on `go.mod`, `STATUS.md`,
`DECISIONS.md` and the proto files. Each agent therefore gets its own git
worktree and branch, created by the owner (the repo rule that agents do not
create branches still holds):

```
cd the repository root (`git rev-parse --show-toplevel`)
git worktree add ../repose-ws/03-hostd -b ws/03-hostd main
cd ../repose-ws/03-hostd && claude      # paste preamble + block 03
```

One worktree per workstream, all branched from the same `main` commit.
When an agent reports done, the owner reviews and merges: `git merge
ws/03-hostd` on main, resolving the shared files (keep both sides of
`go.mod` requires and run `go mod tidy`; concatenate `STATUS.md` and
`DECISIONS.md` entries, renumbering `I-<n>` if two agents used the same
number). Merge the workstreams that own interfaces first (03, 05), then
their consumers. Delete the worktree after merging: `git worktree remove
../repose-ws/03-hostd`.

Nix and Go caches are shared across worktrees (`/nix/store`, `~/go/pkg`),
so parallel builds do not multiply disk use. The store lives on the dev
box's temp disk (`docs/ops/DEV-BOX.md`); keep 40 GB free on `/nix`.

## M1 bring-up (owner runs the apply, then `/ws m1`)

The apply creates paid resources, so it is run by the owner, from `main`,
with the local tfvars in place (`infra/azure/prod/prod.local.tfvars`):

```
cd infra && nix shell nixpkgs#opentofu nixpkgs#azure-cli nixpkgs#gnumake nixpkgs#nixos-anywhere -c \
  bash -c 'make init ENV=prod && tofu -chdir=azure/prod apply -input=false -var-file=prod.tfvars -var-file=prod.local.tfvars'
```

That builds the network, NAT gateway, snapshot storage, Key Vault and the
edge VM, and installs NixOS on the edge through nixos-anywhere (about ten
minutes). Then the M1 integration session (`/ws m1` in
`../repose-ws/m1-integration`) does, in order:

1. Copy `hostdev` to the edge (`nix copy --to ssh://root@<edge-ip>
   'git+file://.?dir=nix#hostdev'`), run `hostdev init --listen 0.0.0.0:443
   --names <edge-ip>` there and `hostdev serve` as a transient unit. The
   edge NSG already opens 443. The token it prints is host-01's join token.
2. Set `repose.host.apiAddr = "<edge-ip>:443"` for host-01 (a per-host
   module passed through `lib.mkHost`, or the host module's default until
   the api exists) so hostd registers with hostdev rather than
   `api.repose.herakraft.co`.
3. Add `"host-01"` to `hosts` in `prod.tfvars`, `host-01 = "<token>"` to
   `join_tokens` in `prod.local.tfvars`, then `make plan` and (owner) apply
   again: nixos-anywhere installs the host through the edge, delivers the
   token, hostd registers, `hostdev status` shows the host.
4. Close the real-host checklist items of 01, 02, 03 and 04: lsblk, /dev/kvm,
   kvm_intel nested, IMDS blocked from a guest, thin pool, bridge, then
   `hostdev create --project todo --class large --closure <guest-system>`
   and SSH into the guest through the edge and host. Record timings in
   `docs/RESEARCH.md` (DECISIONS I-12).

WireGuard between edge and hosts is workstream 06; until it exists the M1
session reaches guests by jumping edge → host → guest.

## M2 bring-up (`/ws m2` in `../repose-ws/m2-integration`)

Wave three is merged (06, 07, 08, 13 and the rest of 11) and the control
plane runs as Coolify resources on the control VM (DECISIONS I-83 to I-91):
`api` answers at `https://api.repose.herakraft.co/healthz` with a Let's
Encrypt certificate, `web` at `https://repose.herakraft.co`, Postgres is the
`repose-postgres` Service, the api migrated itself and created the CA at
first start. Logto is the owner's `accounts.herakraft.co` (I-84) with the
`repose-cli`, `repose-web` and `repose-api` applications in place. What is
*not* yet true, and is this session's work, in order:

1. **Control VM ⇄ edge WireGuard.** The VM's key exists
   (`/etc/wireguard/publickey`); the edge is still the wave-one install
   (sshd on 22, hostdev on 443, no `wg0`). Redeploy the edge from `main`
   (`nix/edge`, workstream 06: gateway on 22, operator sshd on 2222, `wg0`,
   `wgsync`, preview stub, hook ingest), then peer the two per
   `infra/README.md` "Wiring the control plane to the edge" and record the
   hub with `repose-admin edge init`. The conductor runs the apply; you
   prepare the tfvars change and ask.
2. **api-grpc reachable where hostd and the gateway look for it.** The
   design puts gRPC (8443) and `/internal` (8444) on the VM's WireGuard
   address `10.255.255.1`, never on the public IP (`ops/coolify/README.md`);
   the Coolify `api-grpc` app needs its port mappings and the control VM
   must route those to the tunnel. Verify with `openssl s_client` from the
   edge and with the gateway client certificate
   (`repose-admin ca sign-client --name gateway`), then `wgsync` pulls
   `/internal/hosts`.
3. **host-01 registers with the real api.** It is registered with `hostdev`
   on the edge (M1). Settle the bootstrap order, which the docs leave
   circular: a host reaches the api over WireGuard, but learns the hub's
   peer from the api. Candidates: the edge's public key and endpoint in the
   host's Nix config (not secret, like `apiCA` today) so `wg0` is up before
   hostd starts, with the edge accepting the host's first handshake through
   `wgsync` after registration; or a bootstrap path through the edge on the
   VNet. Pick one, record it as a DECISIONS entry, implement it in
   `nix/hosts` and `nix/edge` as needed, drop `apiAddr`/`apiCA`/`bootstrap`
   from `host-01.nix`, issue a join token from the api, and have the
   conductor apply. `repose-admin hosts list` shows host-01 `ready`; stop
   `hostdev` on the edge and note it in STATUS.
4. **Owner-run gate.** You cannot sign in to GitHub. When `repose login`
   and `repose run` will work, write the exact commands for the owner and
   for a second person into STATUS.md and your report, and message the
   conductor. While they run them, verify from the host and the api side
   that each landed in their own guest, that the second cannot reach the
   first's guest (`test/isolation`, 14-security's checks on the real host),
   that sessions were reported to the api, and that the finished-agent
   notification arrived (13). Close the real-host rows of 06 and 07's
   checklists with evidence, and record timings in `docs/RESEARCH.md`.

Rules that apply on top of the preamble: never `tofu apply`, never
`force-unlock`, never touch the Coolify UI yourself; the owner and the
conductor do those, and you tell them exactly what to click or run. Nothing
about the owner's personal server (addresses, keys) goes in any file.
`docs/ops/coolify.md` "Coolify facts that cost a round trip each" is the
list of things already learned the hard way; read it before touching a
Coolify resource.

## M3 bring-up (`/ws m3` and `/ws m3-web`, two sessions)

M2 put the real path together: api and web on Coolify (control VM, a
server of the owner's Coolify, I-83), the edge on WireGuard with the gateway
on 22, host-01 registered with the api over the VNet (I-92). The M3 gate
(`docs/MILESTONES.md`): rolling deploys, Postgres backed up to R2 nightly
with a rehearsed restore, a secret set in the dashboard appears in a guest,
a non-Nix user adds a package from the menu and sees it in their guest
without a reboot. Every checklist row of 05, 08, 10, 12, 13 and 14 that
says "real host", "real guest", "Coolify" or "Logto" is closed in these two
sessions with evidence; rows already closed locally are ticked from the
evidence in the workstream's STATUS lines and commits, not re-run.

### `m3` (Fable 5.1, `../repose-ws/m3-integration`): guests through the api

Owns host-01 and everything on it; nobody else creates guests while it runs.
In order:

1. A project created through the api lands on host-01 and reaches
   `running`; `repose run` from the dev box (the owner's login is in
   `~/.config/repose`, or use the device flow with the owner watching)
   attaches. Base publish (`repose-admin base publish --rev <main sha>`) if
   M2 did not.
2. **Secrets** (`docs/features/secrets.md`, 05 §secrets, 04): set one via
   the api, see the file in the guest's tmpfs with the documented mode,
   delete it, see it gone; the value never appears in api logs, hostd logs
   or the build log.
3. **Menu and Nix** (12 §5, `docs/features/config.md`): add a package from
   the catalog through the api, watch the build log stream, confirm the
   package is in the guest without a reboot; then the fragment edit and
   takeover flow; `kernel_changed` true for a base kernel bump; the
   restricted-eval refusals on the real host (readFile /etc/passwd, import
   <nixpkgs>, fetchurl without a hash); closure cap; GC roots after
   destroy. Record eval and build timings in `docs/RESEARCH.md`.
4. **Notifications** (13's two open rows): each of the five agents produces
   a `completed` event in a real guest; `repose status` and the dashboard
   show it; the email or ntfy delivery arrives (the owner's ntfy URL, asked
   through the conductor).
5. **api resilience on the real path** (05): kill `api-grpc` during an op
   and see hostd reconnect with nothing lost; ops survive an api restart;
   base bump job builds unheld projects and skips held ones; snapshot expiry.
6. **Security on the shared host** (14): every row of the boundary table
   as a test on host-01, fork bomb and memory hog leaving the neighbour
   within limits, audit_log rows for every audited action, operator
   password attempt refused. Coordinate with the conductor before anything
   that could take host-01 down.
7. M1 follow-up (a): `nix/hosts/tests` host-services needs a fake api for
   the repose-register assertion now that the real hostd is installed.

### `m3-web` (Opus 5, `../repose-ws/m3-web`): dashboard, deploys, backups, ops

Does not create guests; project flows that need one wait for `m3`'s step 1
(ask the conductor). In order:

1. **Dashboard against the real Logto and api** (08): sign-in, callback,
   refresh, sign-out at `https://repose.herakraft.co`; settings (timezone,
   email toggle, ntfy URL, test button); account deletion flow; the
   Lighthouse accessibility score on `/projects` and the project page;
   landing page with the install command and the pricing table matching
   `docs/features/pricing.md`. Playwright against the real site where the
   fake-api suite already passes locally.
2. **Rolling deploys** (05, 08): deploy `web` twice while `curl` loops
   against it, then `api` the same way, and record zero failed requests;
   any redeploy of `api` or `api-grpc` is announced to the conductor first,
   because `m3` may be mid-operation on host-01.
3. **Backups**: the R2 token is the owner's (ask through the conductor);
   then `infra/r2` apply, the S3 destination and nightly schedule on the
   Postgres Service, one manual backup, `repose-backup-check` green, and
   the restore rehearsal onto staging's control VM timed and recorded in
   `docs/CHECKLIST.md`.
4. **Observability on the real path** (10): the api's, edge's and host's
   metrics are scrapeable over WireGuard; Fluent Bit on host-01 ships
   journald and guest console logs; the seven dashboards render with real
   data and the eleven alerts load. The Prometheus, Loki and Grafana are the
   owner's (personal server); what they need from the owner (a WireGuard
   peer for the scraper, endpoints) goes through the conductor with the
   exact config to paste.
5. `ops/RUNBOOK.md` rows for 08 and 10; `docs/features/*` match what is
   live; `docs/SECURITY.md` and the privacy and terms passages (14's text
   rows).

Both sessions: the rules of the M2 block apply (no apply, no force-unlock,
no Coolify UI; the owner and the conductor do those on your exact
instructions). Read `docs/ops/coolify.md` "Coolify facts" first.

## Shared preamble

Paste this first, then the workstream block.

```
You are building one workstream of repose in the repository root (`git rev-parse --show-toplevel`).
Read, in order: AGENTS.md, docs/README.md, docs/DESIGN.md, docs/DECISIONS.md,
docs/workstreams/README.md, then your workstream doc and every docs/interfaces/
file it owns or consumes. Do not skim; the checklists reference exact names.

Rules:
- Add your claim line to docs/workstreams/STATUS.md before writing code, and
  update it when you stop, with what is done and what is not.
- Build the entire workstream, not the easy parts. Your doc's section 9
  checklist is the definition of done; docs/CHECKLIST.md applies on top.
  Close each item with the evidence it names, pasted into your final report.
  A passing build closes nothing.
- Interfaces are law. If a contract is wrong, record the change in
  docs/DECISIONS.md under "Made during implementation" as I-<next number>,
  update the interfaces/ doc and the proto in the same commit, and keep the
  old shape accepted for one release.
- Where the producer of an interface you consume does not exist yet, write
  or use the fake named in docs/interfaces/README.md.
- You are in a git worktree on a branch named ws/<nn>-<name> that the
  owner created for you. Commit there in small steps with messages that
  name the workstream. Do not create further branches, do not merge, do not
  push, do not touch main. The owner merges branches into main.
- Never install anything from nix/ on this machine; it is an AMD dev box,
  not a host. nix build and nix flake check are fine.
- Never log prompts, terminal contents, process arguments, environment
  variables, secret values, tokens, certificate bodies, or email.
- Run `just done-check` and `just lint` before reporting. Finish with a
  report listing every checklist item and its evidence, then what is not
  done and why.
```

## Per-workstream blocks

### 01 host-nixos

```
Workstream: docs/workstreams/01-host-nixos.md. Build nix/hosts/ so a fresh
Azure Ubuntu VM becomes a repose host via nixos-anywhere: disko layout,
kernel modules, br-guests, the nftables table in docs/interfaces/host-
conventions.md including the IMDS block and per-guest counters, tc shaping,
LVM thin pool on the data disk, virtiofsd and cloud-hypervisor packages,
WireGuard client, Fluent Bit, node_exporter, the hostd systemd unit, the
join-token path, GC roots directory, hardening. `nix flake check` must pass
and `nix build .#nixosConfigurations.host.config.system.build.toplevel`
must succeed locally. Anything that needs a real host is marked in the
checklist as such; do everything else.
```

### 02 guest-base

```
Workstream: docs/workstreams/02-guest-base.md. Build nix/guest/base/ and
nix/guest/microvm.nix: the platform guest module per docs/interfaces/guest-
conventions.md (dev user, docker, tmux config, sshd trusting the CA with
principals file, agents from nix/overlay/agents with wrappers and hooks,
headless chromium and playwright browsers, Xvfb/x11vnc/noVNC socket-
activated, inotify sysctls, TZ/LANG, guestd unit, secrets tmpfs, base-
version stamp), and the mkGuest function that composes base plus a user
fragment into a microvm.nix Cloud Hypervisor runner with the virtio-fs
store share and the thin-volume overlay. Retire packages/core/flake.nix
into nix/guest/base/tools.nix. `nix build` of a runner with an empty
fragment must succeed locally, and the runner must boot under
cloud-hypervisor on any KVM machine if one is available; if not, say so.
```

### 03 hostd

```
Workstream: docs/workstreams/03-hostd.md. Build cmd/hostd and its internal
packages: Register and the mTLS Session stream with reconnect and command
replay per docs/interfaces/grpc-hostd.md, the guest state machine, every
command (Create, Start, Stop, Destroy, Resize, Build, ApplyConfig,
Snapshot, Restore, UpdateSecrets, SetPrincipals, Exec, Drain) implemented
against LVM, nftables, tc, systemd-run, the Cloud Hypervisor API socket,
virtiofsd and the microvm.nix runner, bbolt state and reconciliation at
start, GC roots, samples every 60 s, console capture, snapshot streaming to
Blob, Prometheus metrics, and the one-host dev driver cmd/hostdev
(DECISIONS I-17). Generate the Go code from proto/ with buf.
Everything that shells out goes behind an interface with a fake so the
state machine is unit-tested without a host; the host-level tests are
marked as such in the checklist.
```

### 04 guestd

```
Workstream: docs/workstreams/04-guestd.md. Build cmd/guestd: the vsock
server per docs/interfaces/vsock-guestd.md with the unix-socket dev mode,
every request (Ping, Freeze with the thaw watchdog, Thaw, Switch, GrowFs,
WriteSecrets, SetPrincipals, SetupProject with tmux, Sample from /proc and
tmux, Exec, Shutdown), the Notify messages, the hook socket at
/run/repose/hooks.sock and the repose-hook helper. Unit-test everything
against a fake filesystem and a real tmux where available.
```

### 05 control-plane-api

```
Workstream: docs/workstreams/05-control-plane-api.md. Build cmd/api and
cmd/repose-admin: every route in docs/interfaces/api.md with Logto JWT
verification, the Postgres schema in docs/interfaces/db-schema.md as
migrations, the gRPC server side of docs/interfaces/grpc-hostd.md with
the fake hostd for tests, the scheduler, SSH CA issue and revoke and the
guest host-key generation (I-3), envelope-encrypted secrets with a fake Key
Vault for tests, config revisions with SSE build logs, snapshots, events
ingest and the notification outbox, meter ingest and hourly rollup,
internal routes for the gateway, rate limits, the full repose-admin
surface named in DECISIONS I-9, Dockerfile with health check for Coolify,
OpenTelemetry wiring. Integration tests run against a real Postgres.
```

### 06 gateway-edge

```
Workstream: docs/workstreams/06-gateway-edge.md. Build cmd/gateway and
nix/edge/: the SSH gateway per docs/interfaces/ssh-gateway.md (certificate
verification, revocation cache, route lookup, terminate-and-redial with a
gateway-issued 5-minute certificate from /internal/gateway-certs, channel
relay for session, port forwards and agent forwarding, host certificate,
session reporting, metrics), wgsync from /internal/hosts, the hook ingest
forwarder, the preview-proxy stub on 443, and the edge NixOS config with
WireGuard hub and firewall. Test the relay end to end with
internal/ca/testca and an in-process SSH server standing in for a guest.
```

### 07 cli

```
Workstream: docs/workstreams/07-cli.md. Build cmd/repose: every command
listed there including events and notify (DECISIONS I-8), Logto login with
PKCE loopback and device-code fallback, project resolution and remote
normalisation per docs/interfaces/cli-config.md, certificate refresh and
SSH config writing per docs/interfaces/ssh-gateway.md, the run sequence
with the git-based sync and dirty-tree refusal, credential file sync per
docs/interfaces/guest-conventions.md, tmux attach and prompt send, open
<port> and --desktop, status, SSE build log rendering, exit codes,
install.sh and goreleaser config for four platforms, shell completion.
Test against internal/fakes/api and a local sshd.
```

### 08 dashboard

```
Workstream: docs/workstreams/08-dashboard.md, and read docs/DESIGN-
LANGUAGE.md before any markup. Build apps/web: Logto SPA login, projects
list with status and cost, project detail with events and signals, secrets,
the config menu from GET /catalog rendering to a fragment plus a raw
fragment editor showing build errors, snapshots, billing card setup and
portal, notification settings with the test button, Dockerfile and health
check for Coolify rolling deploys. Copy the recruiting app's typography,
palette, buttons and fields; do not copy its chip, segmented or card
radios. Test against internal/fakes/api.
```

### 09 billing

```
Workstream: docs/workstreams/09-billing.md. Build the billing package and
its api routes: Stripe customer at signup, SetupIntent card on file, the
$10 trial credit ledger, hourly usage records from usage_hours, the
per-project monthly cap per size, storage and egress lines, monthly
invoices, past_due handling with the 3-day stop and 30-day retention,
webhooks, the reconciliation job, and the test-mode fixture for the
CHECKLIST usage pattern that must match to the cent.
```

### 10 observability

```
Workstream: docs/workstreams/10-observability.md. Build the shared
telemetry package (structured logs with the never-log list enforced by a
test, repose_* metrics, OTel wiring), the Fluent Bit config in nix/hosts,
the Prometheus scrape config and WireGuard peer for the personal Grafana
server, and every dashboard and alert rule listed in docs/CHECKLIST.md as
Grafana JSON and Prometheus rules under ops/grafana/.
```

### 11 infra-opentofu

```
Workstream: docs/workstreams/11-infra-opentofu.md, after the human steps in
docs/ops/AZURE-SETUP.md are done. Build infra/azure: network with NAT
gateway and no inbound, the host module (Intel Dsv5 size from the host_size
variable, default Standard_D16s_v7 per DECISIONS I-14 and I-39, security type
Standard, Premium SSD v2 data disk, cloud-init writing the join token,
nixos-anywhere provisioner), edge VM, Coolify Ubuntu VM with cloud-init,
Blob with lifecycle rules, Key Vault with the wrapping key, the remote
state backend; infra/r2 for the backup bucket. `tofu validate` and `tofu
plan` must be clean; apply only when the owner says so.
```

### 12 nix-config-pipeline

```
Workstream: docs/workstreams/12-nix-config-pipeline.md. Build the fragment
contract and its enforcement: restricted pure evaluation with the exact
nix flags, the allowed inputs, eval and build timeouts, core and closure
caps, error extraction with fragment line numbers, the catalog format and
the menu-to-fragment renderer used by the api, base bump flow with the
hold flag, kernel_changed detection, and a test corpus containing the five
error cases from docs/CHECKLIST.md producing their exact messages.
```

### 13 notifications

```
Workstream: docs/workstreams/13-notifications.md. Build the event pipeline
end to end: per-agent hook installation in the wrappers (Claude Code
Notification and Stop hooks; the documented equivalents or tmux-idle
heuristic for opencode, codex, gemini-cli, pi), the outbox with retries and
dedupe in the api, Resend email and ntfy delivery, user settings, the
notify-test route, and what repose status, repose events and the
dashboard show.
```

### 14 security

```
Workstream: docs/workstreams/14-security.md. Review, do not build features:
verify every isolation claim in docs/SECURITY.md against the code and Nix
config as it exists, write the isolation test scripts from
docs/CHECKLIST.md, audit log coverage, secrets handling, operator access;
write the privacy policy and terms text with the process-sample boundary
and the Anthropic hosted-use statement verbatim. Report findings as a list
with severity and the exact file and line; fix only what is a clear bug.
```
