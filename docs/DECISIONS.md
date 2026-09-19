# Decisions

Every settled decision, the alternatives that lost, and why. Numbered by the
interview round they were made in (R1 to R5) so the transcript can be found.
Add new entries at the bottom under "Made during implementation". To reverse
a decision, add a new entry that references the old one; never edit the old
one.

Format: **id. Title.** Decision. *Rejected:* the alternatives. *Why:* the
reason. *Revisit when:* the trigger, if any.

## Scope

**R1-1. First release is multi-tenant.** The single-user v0.01 (one Ubuntu VM,
home-manager flake) exists and works; the product being built is the
multi-user service. *Rejected:* single-tenant v0 then grow. *Why:* the owner
wants the ambition intact and the architecture questions answered now.

**R1-2. Unit of environment is one microVM per project on shared hosts.**
*Rejected:* one Azure VM per user holding all projects (no isolation between
projects, one runaway agent hurts the rest); one Azure VM per project (boot
time, idle cost, duplicated Nix stores). *Why:* per-project isolation with a
per-host shared store gives both properties.

**R1-3. Git is the exchange channel, plus a one-shot sync of the uncommitted
diff at launch.** *Rejected:* bidirectional file sync (mutagen) fights a
running agent writing files; remote-only editing forces a habit change.
*Revisit when:* users ask for live sync; consider a `repose sync --watch`.

**R1-4. `run` attaches, `run "prompt"` starts an agent, with a prelisted agent
picker defaulting to Claude.** Build both from the start.

**R1-5. Guests are always-on until `stop`.** No idle auto-stop in the first
release; the signals to design one are recorded from day one. *Why:* idle
detection that kills a long agent job is worse than a bill. *Revisit when:*
the recorded signals show what an unattended agent looks like from outside.

**R1-6. No Tailscale for user access.** *Why:* per-user tailnets do not fit a
multi-tenant product. *Replaced by:* R2-5 (SSH CA and gateway) and R4-3
(WireGuard between edge and hosts).

**R1-8 (amended R2).** Secrets: see R2-8.

## Hosts and guests

**R2-1. NixOS guests on Cloud Hypervisor via microvm.nix with the host's Nix
store shared read-only over virtio-fs.** *Rejected:* Ubuntu guests on
Firecracker with a per-host binary cache (saves bandwidth, not disk or boot
time; Firecracker has no virtio-fs); apt-baked images (abandons Nix). *Why:*
the shared store is only real if guests consume it directly, and a declared
guest removes the hardcoded-user problem and the "exactly their config"
problem at once. Cost: commitment to NixOS on host and guest now.

**R2-2. NixOS host installed with nixos-anywhere.** *Rejected:* Ubuntu host
with a hand-rolled daemon doing microvm.nix's job.

**R2-3. Guest disks are host-local with snapshots to Blob; projects are pinned
to a host.** *Rejected:* one managed disk per project (attach limits, attach
latency, per-disk cost); no snapshots. *Revisit when:* host maintenance moves
become frequent.

**R2-17. Host SKU is Intel D64s_v5 with guest images on a Premium SSD v2
managed data disk.** *Rejected:* D64ds_v5 local NVMe (ephemeral; a routine
deallocate loses every tenant's disk unless the snapshot ran); E-series until
memory data says otherwise. *Watch:* the owner asked to keep researching
this. *Revisit when:* snapshots are proven and hot overlay data could move to
local SSD.

**R2-18. Nested virtualization is accepted, gated by a one-day benchmark
before anything else is built.** Threshold roughly 20 percent penalty on CPU,
disk, network. *Rejected:* no benchmark; a second provider from day one.
*If it fails:* hosts go to Hetzner metal, design unchanged (R3-20).

**R3-2. Three size classes: small 2/4, large 4/8, xl 8/16. CPU 2:1
oversubscribed, memory never.** *Rejected:* one or two classes. *Why:* Docker-
heavy test suites are the XL case people pay for; a balloon reclaiming memory
mid-build turns "slow" into OOM.

**R3-3. Config changes apply in place via switch-to-configuration; reboot
only when the kernel or share layout changed.** *Rejected:* reboot always;
refuse while an agent runs. *Why:* losing an unattended agent because the
user added a package is the product's own failure mode.

**R3-6. LVM thin pool, one thin volume per guest, snapshot with a guest-side
fsfreeze over vsock, streamed compressed to Blob. Nightly plus on stop, keep
7.** *Rejected:* qcow2 external snapshots; whole-disk Azure snapshots (one
tenant's restore needs the whole disk mounted).

**R3-20. Azure hosts while credits last; the OpenTofu host module is written
so Hetzner is a second module.** *Rejected:* AWS metal (same cost as Azure,
no credits, and what it gives, Hetzner gives at a tenth); Hetzner now (loses
the credits and the benchmark's information). The owner may get AWS credits
later; that does not change this.

**R5-1. hostd builds each guest's microvm.nix runner on demand and starts it
as a transient systemd unit.** *Rejected:* rewriting the host NixOS config per
guest and rebuilding the host (minutes per tenant action; the host config
becomes a database).

**R5-2. One in-guest Go agent (`guestd`) on vsock only.** *Rejected:* several
scripts driven over SSH from hostd. *Why:* one process, one channel, works
before the guest has network.

**R5-4. User builds: substitutes from cache.nixos.org and the platform overlay
cache; source builds allowed, capped at 30 minutes and 8 cores; evaluation
capped at 60 seconds; per-project closure cap 20 GB.** *Rejected:* cache-only
(breaks every override); no caps (noisy neighbour).

**R5-5. Default volumes 20/40/80 GB thin, resizable up, billed on allocated.
Egress shaped at 200 Mbit/s, 500 GB/month included.**

**R4-5. Base bumps apply automatically weekly (security sooner) with a
per-project hold flag and a changelog line in status.** *Rejected:* opt-in per
bump (agent versions and kernel fixes never reach everyone).

**R4-4. Builds run on the project's host; a central builder plus cache comes
when there is more than one host.**

## Network and access

**R2-5. SSH certificate authority in the control plane, short-lived
certificates, a gateway that routes by login name.** *Rejected:* temporary
keypair injected into guests per run (key distribution into running guests);
per-user WireGuard (a second product to run). Certificate lifetime 12 hours
(R3-9), refreshed silently while the Logto refresh token is valid.

**R2-6. Dev-server access is `repose open <port>` (SSH forward) now; per-
project HTTPS preview URLs later, documented from day one.**

**R3-7. The gateway is custom Go, not OpenSSH ProxyJump.** *Why:* OpenSSH
cannot enforce "this user reaches only this guest" without principals files
on every guest; the Go gateway is also the future preview proxy.

**R3-8. hostd dials out to the API over a mutual-TLS gRPC stream; hosts have
no inbound ports.** *Rejected:* API calling hostd; a broker.

**R4-3. WireGuard between edge and hosts, hosts dial the edge.** *Rejected:*
multiplexing SSH over the gRPC stream; gateway inside the Azure VNet. *Why:*
same outbound-only posture as hostd, and it makes "hosts anywhere" true.

**R4-6. Default deny guest-to-guest and guest-to-host; egress only, shaped;
Azure IMDS blocked from guests.** *Rejected:* allowing cross-project traffic.
*Revisit when:* a user asks to wire projects together; make it explicit.

**R4-13. Gateway and WireGuard hub run on a small NixOS edge VM, not on
Coolify.** *Why:* a Coolify port mapping disables rolling deploys for that
app, WireGuard wants the kernel, and NixOS is already the appliance tool.

## Identity, secrets, policy

**R2-7. Logto (already self-hosted) with the GitHub connector is the identity
provider.** *Rejected:* GitHub OAuth directly; email magic links.

**R5-10. CLI login is authorization code with PKCE and a loopback redirect,
device code as the headless fallback.** Logto supports both.

**R2-8 amended. Tool logins sync from the laptop (gh, Codex, opencode, git
identity); Claude never; named secrets are held centrally with envelope
encryption.** *Rejected:* copying Claude's credentials file (copied refresh
tokens do not refresh, issue closed as not planned); central storage of any
tool login (Anthropic's terms require each user to authenticate with their
own credentials on hosted platforms and forbid third-party reuse of
subscription OAuth).

**R2-15. Claude logs in inside the guest by default; a setup token stored as
a named secret is the headless fallback.** *Why:* the default keeps Remote
Control, which is the "check from my phone" feature.

**R3-10. Named secrets: envelope encryption in Postgres, DEKs wrapped by a Key
Vault key. Disks rely on Azure managed-disk encryption.** *Rejected:* one Key
Vault secret per value (slow, per-operation pricing); per-project LUKS now.
*Revisit when:* you want to make a claim about operator access to tenant
disks.

**R2-10. Egress is unrestricted but shaped; abuse control is card on file plus
GitHub-linked identity plus recorded process samples.** *Rejected:* an egress
allowlist (agents need arbitrary egress).

**R5-3. Process samples contain process names, per-process CPU, memory and
network bytes, sampled every minute. Never arguments, environment, paths or
terminal contents.** The privacy policy states this in the same words.

## Agents and environment

**R2-11 + R2-13 + R2-14 + R2-16. Agents: Claude Code, opencode, Codex CLI,
Gemini CLI, pi. Browser: headless Chromium with Playwright MCP and
chrome-devtools-mcp, plus on-demand Xvfb and noVNC. Laptop-bound MCPs
unsupported at first, `repose mcp forward` planned. Notifications: platform
hooks for every agent plus each agent's own features.** *Rejected:* a longer
agent list (unpackaged tools become packages you maintain); a laptop Chrome
bridge first (only works while the laptop is open, the case being escaped).

**R3-19. Agents come from a platform-owned overlay bumped on a schedule, not
nixpkgs.** *Why:* nixpkgs lags Claude Code by weeks and Gemini CLI by months;
owning the overlay means the platform decides when a tenant's agent changes.

**R3-13. Fixed guest user `dev`, passwordless sudo, project at
`/home/dev/<project>`.** *Rejected:* platform username as Unix user (per-user
values back in the Nix config); `/workspace`. *Why:* MCP configs key on
absolute paths; Claude Code refuses to skip permissions as root.

**R3-14 + R4-10. A prompt starts the agent's TUI in a tmux window named after
the agent; a second `run` with a prompt opens another window with a warning.**
*Rejected:* headless print mode to a log (nothing to attach to); refusing a
second agent; automatic worktrees.

**R3-12. Sync refuses if the guest tree is dirty and offers stash or discard.**
*Rejected:* silently overwriting (clobbers an unattended agent's work).

**R3-11. Projects are keyed on git remote plus user with `--name` override;
nothing committed to the repo.**

## Control plane and operations

**R2-4 + R3-4. Go for api, hostd, gateway, guestd and CLI; Postgres. The
control plane is decoupled from Azure: containers on Coolify, only compute
and storage are Azure-specific.** *Rejected:* TypeScript everywhere; Rust;
a compose monolith in prod (no rolling updates).

**R4-2 + R5-11. Control plane on an Ubuntu LTS VM in Azure running Coolify,
Logto, api and dashboard as single-container apps; Postgres Coolify-managed.**
*Rejected:* NixOS for that VM (Coolify rejects NixOS as a host or managed
server, verified 2026-09-17, open issue since 2024); Azure Database for
PostgreSQL (an Azure dependency in the state store).

**R4-14. Postgres backups to Cloudflare R2.** *Rejected:* Azure Blob behind an
s3proxy (a translation proxy in the backup path). Coolify only backs up to
S3-compatible targets.

**R3-1. Provisioning is OpenTofu for Azure resources with nixos-anywhere as a
provisioner; hostd self-registers.** *Rejected:* Azure SDK calls from the API
(later, for automatic capacity); az CLI scripts.

**R3-5. Capacity is added manually at an 80 percent memory alert.**

**R2-9. Observability reuses the existing Loki, Grafana and Fluent Bit; add
Prometheus there; OpenTelemetry in the control plane with no traces backend
yet. Record everything non-invasive.**

**R2-12 + R3-16 + R4-7 + R4-8. Stripe from day one, card required before the
first guest, $10 credit trial. Meters: guest-hours by class, volume GB-months,
egress GB. Hourly rate with a monthly cap per project equal to the flat price
(49/99/199).** *Rejected:* deferring billing (the owner asked why not bill
now; the abuse argument is handled by the card, and invoices are the only
real test of the meters); flat fee that charges stopped guests; per-account
hour bundles.

**R4-11. Retention: destroy deletes the volume and keeps the last snapshot 30
days; cancellation stops guests, keeps snapshots 30 days.**

**R4-12 + R4 domain note. The name is `repose` everywhere. Hosted under
`herakraft.co` (`repose.herakraft.co`, `api.repose.herakraft.co`,
`ssh.repose.herakraft.co`) until it graduates to its own domain.** *Why:*
velocity and clear ownership; an apps dashboard at `www.herakraft.co` lists
side projects.

**R5-6. No teams in the first release.**

**R5-7. One repository, one Go module, `nix/` and `infra/` alongside the
existing Turborepo.**

**R5-8. Milestone order: benchmark, then hostd and guestd with your own
projects over WireGuard, then edge and auth, then Coolify control plane and
dashboard, then billing, with observability from milestone two onward.** The
owner then asked for all of it to be built rather than a v0.01 rebuild, with
parallel agents; `MILESTONES.md` keeps the gates, `workstreams/` gives the
parallel split.

## Made during implementation

Add entries here as `I-<n>. Title.` with the workstream, the decision, the
alternatives and why. An implementation decision that changes an interface
must also update the `interfaces/` doc in the same change.

**I-1. Gateway terminates and re-dials with a gateway-issued 5-minute
certificate.** (06) *Rejected:* raw TCP relay after auth (the login name is
only known after the client's key exchange completes); falling back to the
client's forwarded agent (kept out of the first release so the guest sshd
check is uniform). Interface: `ssh-gateway.md`, `api.md` (`/internal/gateway-certs`).

**I-2. The gRPC listener for hosts runs as a separate Coolify app
`api-grpc` from the same image with `--mode grpc`.** (05) *Why:* Coolify's
proxy only handles HTTP; a port mapping on the HTTP app would cost it rolling
deploys. The gRPC app restarts with seconds of stream loss that hostd's
reconnect absorbs.

**I-3. The api generates each guest's sshd host key and Host-CA certificate
and passes them in `CreateGuest` and `Restore`.** (05) *Rejected:* hostd
generating keys and asking the api to sign (an extra round trip on the
create path, and hostd holding a signing client). Interface: `grpc-hostd.md`.

**I-4. Internal routes `GET /internal/hosts`, `POST /internal/gateway-certs`,
`POST /internal/events`.** (06) The edge syncs WireGuard peers from the host
list every 30 s; hook events can reach the api over HTTP through the edge
when guestd is unavailable, deduped with the vsock path. Interface: `api.md`.

**I-5. `Heartbeat.draining` and `ApplyResult.reboot_required`,
`ApplyConfig.force_reboot`.** (03) A drained host rejects placements while
serving everything else; a kernel-changing apply does nothing until the user
confirms. Interface: `grpc-hostd.md`.

**I-6. Preview hostnames carry the handle:
`<port>-<slug>-<handle>.repose.herakraft.co`.** (features) *Why:* slugs are
unique per user, not globally. Not built in the first release.

**I-7. `POST /me/notify-test`, and the SSE build-log route accepts
`?access_token=`.** (08, 13) Browsers cannot set headers on EventSource; the
token is never logged and no other route accepts it. Interface: `api.md`.

**I-8. CLI gains `events` and `notify set|test`.** (13) Interface:
`DESIGN.md` §10, `07-cli.md`.

**I-9. The runbook's `repose-admin` surface is the required admin CLI.**
(05, ops) Subcommands named in `ops/RUNBOOK.md` and `11-infra-opentofu.md`
(hosts add/drain/retire/reconcile/mark-lost, projects move/restore/restart,
users suspend, billing rollup/resync, base publish, operator-cert, edge
init, ca sign-host, db migrate/rollback/verify, audit) are part of
workstream 05's checklist.

**I-10. Guest sshd material travels as explicit `CreateGuest` fields and lands
in the guest's secrets tmpfs under reserved names.** (02, 03, 05) hostd writes
`host_key`, `host_cert` and `ssh_ca_pub` to `/run/repose/secrets/` as
`ssh_host_ed25519_key`, `ssh_host_ed25519_key-cert.pub`, `user_ca.pub`, which
the guest base's sshd config reads. The api rejects those names on `PUT
/secrets`. *Why:* one delivery mechanism in the guest, one explicit contract on
the wire.

**I-11. Warning kinds are enumerated.** (04, 12) guestd: `disk_high`,
`inotify_exhausted`, `docker_down`, `freeze_timeout`, `store_path_missing`.
hostd: `pool_high`, `store_high`, `build_queue_deep`, `cache_unreachable`,
`guestd_lost`, `freeze_timeout`. Interfaces: `vsock-guestd.md`, `grpc-hostd.md`.

**I-12. The M0 benchmark is deferred; the first M1 host measures itself.**
(owner, 2026-09-17) The owner chose not to run the standalone benchmark.
Hosts are still Intel `D64s_v5` with security type Standard, and
workstream 03's checklist records build, Docker and clone timings from the
first real host into `RESEARCH.md`. R3-20's Hetzner fallback remains the
escape if those numbers are bad. `00-benchmark.md` stays as the procedure
to run if a host ever needs to be compared.

**I-13. Dashboard design language is the recruiting app's, minus its
non-text controls.** (08) `DESIGN-LANGUAGE.md` lists what to copy (Noto Serif
headings, zinc palette, single blue accent, borders not boxes, button and
field classes) and what to replace (chip and segmented and card radios).

**I-14. Pre-launch host is `Standard_D16s_v5`; the host size is a variable,
not a constant.** (owner, 2026-09-17) The owner will test alone for about a
month with at most 20 projects. Any Intel Dsv5 size supports nested
virtualization, so the design is unchanged; the data disk starts at 512 GB
Premium SSD v2 instead of 2 TB, and the quota request drops to 64 vCPUs in
the family. Growing to `D64s_v5` at launch is an in-place resize (deallocate,
resize, start) because guest volumes are on the managed disk. *Rejected:*
starting on `D64s_v5` (about $2,240 a month of credits spent on empty
capacity); an AMD or B-series size (no or unmeasured nested virt).

**I-6. Guests set `NPM_CONFIG_PREFIX=/home/dev/.npm-global` and put its
`bin` on `PATH`.** (02) npm's default global prefix is the nodejs derivation
itself, so `npm i -g` in a guest fails with EACCES on the read-only store.
*Rejected:* telling users to package everything through config fragments
(agents run `npm i -g` on their own and would hit the error unprompted).
Interface: `guest-conventions.md` "Environment".

**I-15. The product is named Repose.** (owner, 2026-09-19) Replaces
`factory` everywhere: CLI binary, Go module `github.com/heracraft/repose`,
proto packages `repose.*`, config directory `~/.config/repose`, paths under
`/run/repose`, `/var/lib/repose`, `/etc/repose`, env `REPOSE_*`, metrics
`repose_*`, systemd units `repose-*`, hostnames `repose.herakraft.co`,
`api.repose.herakraft.co`, `ssh.repose.herakraft.co`. *Why:* the word says
what the product does (you rest, the environment does not) and no software
trademark or dev-tool product holds it. *Known collisions, accepted:* dead
`repose` packages on npm, PyPI and crates.io (publish scoped or not at
all); an Arch Linux repo tool that ships a `repose` binary (the installer
puts ours first on PATH and warns if another is found, see 07-cli); Rackspace's
dormant OpenRepose API proxy; a supplement brand at getrepose.com. `repose.run`
was free on 2026-09-19 and should be registered. The on-disk checkout may
still be called `factory`; nothing in the repo depends on the directory
name. Supersedes R4-12's naming half.

**I-16. Stripe is not needed until milestone M4; accounts can be billing-
exempt.** (owner, 2026-09-19) `users.billing_status` gains the value `exempt`,
set only by `repose-admin users exempt <handle>`. An exempt user passes every
"card required" and "trial depleted" check, accrues `usage_hours` rows like
anyone else (so the meters are exercised), and is never pushed to Stripe.
The api starts without `STRIPE_*` variables set; billing routes return
`503 billing_disabled` until they are. Workstream 09 removes nothing here:
exempt stays as the operator and design-partner path. *Why:* the owner tests
alone for a month; wiring Stripe before there is anything to bill is work
done in the wrong order.

**I-17. hostd ships a one-host dev driver, `cmd/hostdev`.** (03) The M1 gate
("your own projects run as guests, driven by a local client") needs the api
side of the gRPC contract without the api. `hostdev` is that: one host, one
operator, a JSON state file, every command as a subcommand, build logs and
samples printed. It stays as the break-glass tool for a host that has lost
the api. *Rejected:* building the api first (puts auth, Postgres and Logto on
the critical path to the first running guest).

**I-18. The state store and its resource group are created outside the
environment's apply.** (11) `repose-prod` and the storage account
`reposetfstate3912` inside it were created by hand on 2026-09-19
(`ops/AZURE-SETUP.md` steps 4 and 5) and are read by the environment roots as
a data source, never managed by them. `infra/bootstrap` declares the same
shape — resource group, account with versioning and soft delete, private
`tfstate` container — so a second environment is one apply, and takes
`state_account_name = null` for an environment that keeps its state in another
one's account under a different key (staging does). *Rejected:* importing
production's state account into `infra/bootstrap` now (a corrupted bootstrap
state could then destroy the state of every other root; the import commands
are written down in `bootstrap/main.tf` for the day that trade looks
different); a separate resource group for state, as workstream 11 §2
originally said (it would have meant a second group to protect and a second
one to remember, for no isolation that the `prevent_destroy` on the account
does not already give).

**I-19. The join token reaches a host over SSH after the install, not through
cloud-init.** (11) `docs/workstreams/11-infra-opentofu.md` §2 described
cloud-init writing `/run/repose/join-token`. It cannot work: cloud-init runs
on the Ubuntu image, and nixos-anywhere kexecs and replaces that system
minutes later, so `/run` is a fresh tmpfs by the time hostd starts. The token
is therefore written by a provisioner that connects to the installed NixOS
system through the edge, as file content rather than as a command-line
argument, and `hostd` is restarted. *Why this is better than fixing it with
`--extra-files`:* `custom_data` stays in the Azure VM model and is readable
through IMDS for the life of the VM, and `--extra-files` would put a
single-use secret on the persistent root disk. *Interface:*
`interfaces/host-conventions.md`, whose `/run/repose/join-token` row said
"from cloud-init". The path, the mode and the one-shot semantics are
unchanged, so nothing that reads the file changes; the runbook's
"Host never registered" recovery was already this exact mechanism by hand.

**I-20. Credentials stay human steps: the api's Entra app registration and
the R2 API token.** (11) `infra/` creates the Key Vault, the wrapping key and
an access policy for the api's service principal given its object id
(`api_identity_object_id`, null until it exists), and creates the R2 bucket
and its lifecycle rule. It does not create the app registration, its client
certificate, or the R2 token. *Rejected:* the `azuread` provider plus
`tls_private_key` (the api's private key would sit in the state file in clear
text for the life of the environment); `cloudflare_api_token` (same, for the
backup credential). *Why:* `ops/AZURE-SETUP.md` already draws this line —
one-time human actions that OpenTofu cannot do, or that agents should not be
trusted to do with the owner's money and identity — and a credential in state
is a credential in every backup of that state.

**I-21. `.terraform.lock.hcl` is committed.** (11) It was in `.gitignore`.
A dependency lock file that is not committed means CI resolves whatever
provider version shipped that morning, so the plan a reviewer reads and the
plan CI runs can differ. The files are locked for `linux_amd64`,
`darwin_arm64` and `darwin_amd64` so the owner's laptop and the dev box agree.
*Rejected:* pinning exact versions in `required_providers` instead (it pins
the version but not the checksum, and it has to be edited in four roots).

**I-22. The edge VM is `Standard_D2s_v5` and its NSG opens 22, 443,
51820/udp and 2222.** (11) `docs/workstreams/11-infra-opentofu.md` §2 said
`Standard_B2s` and "inbound 22/tcp and 51820/udp"; the size note at the top
of the same document, added with I-14, says `Standard_D2s_v5`. The later note
wins, and the burstable size is the wrong shape anyway: the thing that would
throttle when its credits run out is every user's SSH session. The port list
comes from `workstreams/06-gateway-edge.md` §5.1, which is the document that
owns the edge's listeners: 22 is the user gateway, 443 the preview-proxy
stub, 51820/udp the WireGuard hub, and 2222 the operator sshd — restricted to
the operator address list and the VNet, because every host provisioner jumps
through it and hosts have no public IP.
