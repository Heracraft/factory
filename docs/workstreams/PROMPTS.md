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
variable, default Standard_D16s_v5 per DECISIONS I-14, security type
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
