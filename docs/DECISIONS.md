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

**I-36. Guests set `NPM_CONFIG_PREFIX=/home/dev/.npm-global` and put its
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

**I-18. Host runtime configuration has one input, `host.json`; the host
owns the network files, the bridge isolation lives in an nftables `bridge`
table, and guests are cut off from every private range.** (01)
Recorded because the docs disagreed with each other or with the kernel:

- hostd writes `host.json` only and restarts `repose-host-net.service`
  (which renders `wg0.conf`, `host.env`, `host_ca.pub`, `sshd.conf` and
  the bridge address). 03-hostd §5.2's "hostd writes
  `/var/lib/repose/hostd/wg0.conf` and restarts `wg-quick@wg0`" is
  superseded; `wg-quick-wg0.service` reads `/run/repose/wg0.conf`.
  *Rejected:* two writers of WireGuard config (a rotation by hostd and a
  boot render by the host would race).
- The nightly timer calls `hostd snapshot-all` (03's name), not `hostd
  snapshot --all --reason scheduled` (01's text). 03 owns the binary.
- Guest-to-guest traffic on the same bridge never traverses the `inet`
  forward hook, so the documented `10.64.0.0/12` drop cannot isolate
  tenants by itself. A `bridge repose` table drops all switched frames
  and admits only ARP and IPv4 from `(mac, ip, tap)` tuples hostd
  registers in its `guests` set; taps are attached `isolated on learning
  off flood off` with a static FDB entry. The `inet` drop stays for
  routed traffic. *Rejected:* `br_netfilter` (routes bridged frames
  through iptables hooks, slower and a global switch); trusting port
  isolation alone (no anti-spoofing).
- hostd's per-guest objects live in `guest_dyn` and in the `guests` set,
  and the host's ruleset is applied by flushing only its own chains, so a
  `nixos-rebuild switch` never loses a running guest's counter or
  admission. 03's "nft add element repose guests { <ip> . tap }" becomes
  the bridge-table element above.
- Guests are dropped to every private range (`10/8`, `172.16/12`,
  `192.168/16`, `100.64/10`, `169.254/16`) and to the Azure wire server
  `168.63.129.16` (it serves extension protected settings), not only to
  IMDS and `10.64.0.0/12`. DESIGN §7's "everything else is egress to the
  internet" is the intent; the VNet, the WireGuard mesh and the Coolify
  VM's private address are not the internet.
- `kernel.unprivileged_userns_clone` is a Debian patch; the NixOS kernel
  equivalent `security.allowUserNamespaces` is set instead.
- The guests slice reserve follows DESIGN §4 (8 GiB below 128 GiB of RAM,
  16 GiB above), computed at boot, rather than 01's flat 16 GiB.
- `system-features = kvm` and membership of the `kvm` group are required
  on the dev box for the host VM tests (`docs/ops/DEV-BOX.md`).

Interfaces: `host-conventions.md` rewritten with the `host.json` shape,
the tap attach sequence, both tables, the store export path and the
`hostd` subcommand contract. The old inet-only rules are not kept: no
host has been provisioned yet.
**I-19. The state store and its resource group are created outside the
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

**I-20. The join token reaches a host over SSH after the install, not through
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

**I-21. Credentials stay human steps: the api's Entra app registration and
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

**I-22. `.terraform.lock.hcl` is committed.** (11) It was in `.gitignore`.
A dependency lock file that is not committed means CI resolves whatever
provider version shipped that morning, so the plan a reviewer reads and the
plan CI runs can differ. The files are locked for `linux_amd64`,
`darwin_arm64` and `darwin_amd64` so the owner's laptop and the dev box agree.
*Rejected:* pinning exact versions in `required_providers` instead (it pins
the version but not the checksum, and it has to be edited in four roots).

**I-23. The edge VM is `Standard_D2s_v5` and its NSG opens 22, 443,
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

**I-24. The control-plane VM is not created until wave 3.** (11, owner,
2026-09-19) `coolify_count` defaults to 0 in both environment roots. The api,
the dashboard and Logto are workstreams 05 and 08; until they exist the VM
bills about $180 a month for nothing, while the edge and the first host are
worth paying for early, because installing NixOS onto an Azure VM with
nixos-anywhere is the riskiest unproven step in the plan and workstream 01's
data-disk device path stays unverified until a real install happens.
*Rejected:* creating it with everything else (the original shape of workstream
11 §2) and stopping it by hand (a deallocated VM still bills its 256 GB
Premium OS disk, and a VM that exists is a VM somebody configures). Setting
`coolify_count` back to 0 after the VM exists destroys it and its OS disk,
Postgres included; the retention that matters is the R2 dump.

**I-25. The installer reaches a host through the edge, never through a
temporary public IP.** (11) `nixos-anywhere`, the post-install checks and the
join-token delivery all connect to the host's private address with the edge as
an SSH jump host, and the module graph makes a host depend on the edge being
installed. *Rejected:* giving the host a public IP for the length of the
install and removing it afterwards. *Why:* a public IP on a host needs an
inbound rule on the hosts subnet, which is the one thing
`infra/policy/tfsec` forbids and `DESIGN.md` §4 and §7 promise never exists;
the window is not short (kexec, disko, closure copy and reboot is about ten
minutes) and what sits in it is a stock Ubuntu image accepting root SSH; and
the edge path is the same one the runbook's "Host never registered" recovery
already used, so the recovery path is exercised by the happy path rather than
first tried in an incident. *Cost:* the edge must exist and be reachable
before the first host, and its operator sshd must be listening on
`edge_operator_ssh_port`. Until workstream 06 moves it, `nix/edge` serves sshd
on 22, so the first apply sets `edge_operator_ssh_port = 22`.
**I-26. Commands carry what hostd cannot keep: StartGuest repeats the
delivery fields, CreateGuest and Restore name the user, slug and remote,
Restore names the closure, Exec carries an audit id, StopResult carries
the snapshot's blob path.** (03) hostd holds secrets and sshd material in
memory only (secrets have three homes and the host disk is not one), so
after a hostd restart a `StartGuest` with only `guest_id` would boot a
guest without its secrets or host key. `StartGuest` now accepts the same
optional fields as `CreateGuest` (`secrets`, `env`, `ssh_ca_pub`,
`principals`, `hooks_config`, `host_key`, `host_cert`, `project_json`); the
api sends them on every start, and the old shape (guest id only) still
works while hostd has the values cached. `CreateGuest` and `Restore` gain
`user_id` (the snapshot path is `<user_id>/<project_id>/<ts>.img.zst`),
`project_slug` and `remote_url` (guestd's `SetupProject` needs them) and
`project_json`; `Restore` gains `system_closure` (the doc said "the closure
the api passed" but the message had no field). `Exec` gains `audit_id`,
required, because the doc says Exec is only accepted with one. `StopResult`
gains `blob_path` and `bytes` because hostd never knows the api's
`snapshot_id`. *Rejected:* an encrypted secrets cache on the host disk (a
fourth home for secrets); hostd asking the api for secrets over the stream
(a request channel the contract does not have). Interface: `grpc-hostd.md`,
`hostd.proto`.

**I-27. hostd launches Cloud Hypervisor directly from the guest's system
closure; no per-guest microvm.nix runner is built.** (03) The NixOS
toplevel already carries `kernel`, `initrd`, `init` and `kernel-params`;
hostd renders the `cloud-hypervisor` argv from them plus the guest record
(tap, MAC, CID, volume, class) and writes it to
`/var/lib/repose/guests/<id>/ch.args`. microvm.nix's runner would only wrap
the same values, and building one per guest means evaluating the whole
NixOS system on every start (tens of seconds, against the 5 s start in
DESIGN §5), while one runner per base cannot take per-guest arguments.
The guest side (virtio-fs tag `ro-store`, root on the disk, the writable
store overlay) stays in 02's module; `mkGuestRunner` remains for VM tests
and `nix flake check`. The kernel command line hostd adds is `init=`,
`console=ttyS0` and the static `ip=` line 02 documents. *Rejected:* runner
per guest at start (eval cost); one runner per base with arguments
(microvm.nix does not produce one). Interface: `host-conventions.md`
(`ch.args` replaces `runner`).

**I-28. The platform flake takes the user fragment as a non-flake input
named `fragment` and exposes `guestSystem`; hostd fetches base checkouts
with git.** (03, for 12) Pure evaluation forbids reading any absolute path
outside the flake's own source, including store paths given as literals,
so a fragment file cannot be passed with `--apply` or `--arg`. It is
passed as `--override-input fragment path:/var/lib/repose/builds/<rev>`,
which Nix copies into the store and lets the flake read as
`${fragment}/fragment.nix`. Error locations come out as `fragment.nix:L:C`,
which is what the error mapping parses. A base checkout lives at
`/var/lib/repose/base/<base_ref>` and hostd clones the repository there
with `--base-repo-url` when it is missing. The exact contract is
`docs/interfaces/nix-build-contract.md`. *Rejected:* `--impure` (opens
environment and path access to fragments); tarballs delivered in `Build`
(a 30 MB message per build).
**I-29. Two more guestd warning kinds: `oom` and `tmux_down`.** (04) I-11
enumerated five guestd kinds, but `04-guestd.md` §5 and §6 and
`02-guest-base.md` §6 each describe a condition outside that list: the kernel
killing a process for memory, and no tmux server running for `dev`. Both are
things the user sees as an agent that vanished or a `repose attach` that finds
nothing, so both are worth a notification. *Rejected:* folding them into
`disk_high`-style generic text (a kind is what the dashboard and the runbook
key on); dropping them (the two docs that describe them would then be wrong).
The `oom` detail carries the killed process's *name*, which is the one thing
already on the allowed side of the sampling boundary (R5-3). Interface:
`vsock-guestd.md`. Note also that `04-guestd.md` §5 writes the disk kind as
`disk_90`; the enumerated name is `disk_high` and that is what the code uses.

**I-30. `WriteSecrets` carries the whole set, and validates before it
writes.** (04) The request replaces the guest's named secrets: a secret on the
tmpfs that is absent from the list is removed, and `secrets.env` is rewritten
from the list. *Rejected:* treating the list as a partial update (then `repose
secrets rm` never reaches a running guest, and a revoked token keeps working
until the next stop); removing reserved names the same way (they arrive on the
create path, not the secrets path, so they are exempt). Validation of every
name and size happens before the first write, so a rejected batch leaves the
guest exactly as it was. Interface: `vsock-guestd.md`.

**I-31. `Sample` serves the tmux and Docker signals from a 5 s cache, and
carries a `partial` flag.** (04) `04-guestd.md` §5 budgets a sample at under
20 ms and §6 says an over-budget sample returns partial data; forking `tmux
list-windows` and `tmux list-clients` on the call costs more than the whole
budget on its own. A background watcher refreshes them every 5 seconds, which
is also what the agent-state machine and the 90 s pane-idle heuristic need to
run on; `Sample` walks `/proc` fresh and reads the rest from memory. Measured:
4.5 ms for 300 processes. `SampleResult` gains `bool partial = 3`, set when a
signal is missing rather than zero, so hostd can tell "no sessions" from "not
known". *Rejected:* forking on the sampling path (over budget, and it competes
with the agent for a 2-vCPU guest); dropping the accuracy claim (the signals
decide the idle policy later, and a signal that is silently stale is worse
than one that says so). Interfaces: `vsock-guestd.md`,
`proto/repose/guestd/v1/guestd.proto`.

**I-32. `guestd call` is the client side of the vsock contract, in the same
binary.** (04) The NixOS VM test and an operator on a guest that has lost
hostd both need to send a request and read the response; hostd is the only
other client and it is a different workstream's binary. `guestd call <request>
[json]` dials the dev socket or a vsock CID, prints the response as JSON, and
exits non-zero on an error response. *Rejected:* a separate test-only binary
(a tool that exists only in tests is a tool nobody maintains); waiting for
hostd (the VM test is 04's checklist item, not 03's). Interface:
`vsock-guestd.md` "Dev mode and the client", `ops/RUNBOOK.md`.
**I-33. The on-demand desktop is display `:99`, socket-activated, with a
per-start password file.** (02) `features/browser.md` said `:1` and
`workstreams/02-guest-base.md` said `:99`; the module uses `:99` (the
conventional Xvfb display, never taken by a real seat) and the feature doc
is corrected. `repose-novnc.socket` on 127.0.0.1:6080 pulls in websockify
(6081), x11vnc (5900), openbox and Xvfb through `systemd-socket-proxyd`
because websockify cannot inherit a listening socket; a per-minute check
stops the chain after 30 minutes without a client and `systemctl start
repose-desktop-idle` stops it now. x11vnc gets a fresh 8-character password
at every start, kept in `/run/repose/desktop/vnc-password` (0600 dev) for
the CLI to print; noVNC is only reachable through the SSH forward, so the
password is defence in depth. *Rejected:* `Accept=yes` per-connection
websockify (noVNC's page makes several requests, each would fork a server);
no password (a stray forward on a shared laptop would expose the desktop).

**I-34. `mkGuestRunner` takes every per-guest value at run time; the
system closure is guest-independent.** (02, 03) `workstreams/02-guest-base.md`
listed `guestId, ip, gatewayIp, cid, volumeDevice, vcpu, mem` as
evaluation arguments. Baked in, they would put the address allocation
before `Build` (which produces `system_closure` per revision, before
`CreateGuest` allocates an ip), make one closure per guest instead of per
revision, and make `kernel_changed` a per-guest comparison. The function
still accepts those attributes as defaults, but the runner's `bin/run`
takes them as arguments (contract in `interfaces/guest-conventions.md`
"Runner contract"), the kernel line carries `ip=` and the guest's
systemd-network-generator applies it. hostd builds one runner per base
version and revision and starts any guest from it. The vsock device is a
unix socket `vsock.sock` in the guest's state directory (Cloud Hypervisor
implements vsock in user space; the host's `vhost_vsock` module is not
involved), added to `interfaces/host-conventions.md`. *Rejected:*
`config.microvm.declaredRunner` as the output (its script has the sockets,
tap and volume fixed at evaluation).

**I-35. sshd material: reserved secrets at `/run/repose/`, symlinked into
`/etc/ssh/`, a throwaway key until delivery, reload re-reads.** (02, 04)
Three docs disagreed on where `user_ca.pub` and the host key live
(`/run/repose/secrets/`, `/run/repose/`, `/etc/ssh/`). guestd writes the
reserved names to `/run/repose/` (04's design), `/etc/ssh/` holds symlinks
to them so `sshd_config` reads the paths `interfaces/ssh-gateway.md`
shows, and the api keeps rejecting the reserved names on `PUT /secrets`
(I-10). Because `Ready` is sent once sshd listens and the secrets follow
`Ready`, sshd's preStart generates a throwaway ed25519 key when none was
delivered and an empty CA file (trusts nobody); the guest's sshd unit gets
an `ExecReload` (`SIGHUP`, which re-execs sshd) so guestd's `systemctl
reload sshd` after `WriteSecrets` and `SetPrincipals` picks up the real key,
certificate and CA. *Rejected:* delaying sshd until the secrets arrive (a
guest whose hostd died before delivery would have no way in at all).

**I-37. Two vsock RPC implementations exist for one release.** (merge of 03
and 04, 2026-09-19) 03 and 04 each wrote `internal/vsockrpc` and a guestd
fake against the same contract, with the same uvarint-length protobuf
framing, so they interoperate on the wire. 04's stays as the shared
`internal/vsockrpc` (guestd is the canonical server side); 03's moved to
`internal/hostd/vsockrpc` and `internal/hostd/fakeguestd`. hostd should
migrate to the shared package when it is next touched; the M1 integration
session proves the two speak to each other.

**I-38. Generated protobuf code is tracked and also regenerated in the Nix
sandbox.** (merge, 2026-09-19) `internal/gen/` is committed so `go build`
works without buf; `nix/packages.nix` regenerates it with local plugins
inside the build so a stale checkout cannot ship stale stubs. One
`packages.nix` builds every Go binary (guestd, repose-hook, hostd, hostdev)
from one vendor hash.

**I-39. Sizes are Intel v7 (Granite Rapids): host `Standard_D16s_v7`, edge
`Standard_D2s_v7`, control plane `Standard_D4s_v7`, launch host
`Standard_D64s_v7`.** (owner's first apply, 2026-09-20) The first apply
failed with `SkuNotAvailable`: this subscription has every v5 and v6
general-purpose size marked NotAvailableForSubscription in East US and East
US 2, all zones (`az vm list-skus --all`), which is a subscription-level SKU
gate, not capacity. The v7 families are unrestricted in all three zones with
a 350 vCPU quota each already granted. Verified against the size pages:
Intel Xeon 6, x86-64, nested virtualization Supported, Gen2 only, security
type Standard allowed by setting it explicitly, NVMe disk controller only.
Consequences: disks are `/dev/nvme0n1` (OS) and `/dev/nvme0n2` (data LUN 0)
instead of `/dev/sda` and the SCSI udev path; the host module's validation
accepts `Standard_D<n>(l|d|ld)?s_v[567]`; cost is about $772 a month for the
host and $96 for the edge, about 40 percent more than the v5 figures in
`PRICING.md`, which now describe launch economics on a v5 reservation or
Hetzner. R2-17's AMD exclusion stands: `a`-sizes stay out. *Rejected:*
requesting v5 SKU enablement through support (days, uncertain); another
region (same gate); Hetzner now (R3-20's fallback remains available).

**I-40. A production host is a named configuration; the api CA and the
snapshot target are host module options; a host registered by `hostdev`
comes up without WireGuard or a Host CA.** (m1 integration, 2026-09-20)
Bringing host-01 up against `hostdev` on the edge (I-17) found four gaps
between the merged host configuration and a real host:

- `hostd.service` passed no snapshot target, and hostd exits at startup
  without one (`hostd: set --snapshot-dir or --blob-url`). The host module
  gains `repose.host.snapshots.{blobUrl, container, identityClientId,
  localDir}`; the unit passes Blob when `blobUrl` is set and
  `--snapshot-dir` otherwise, with a build warning on an Azure host that
  has no Blob account. *Rejected:* defaulting the Blob URL in the module
  (the account name is an infra value; see `prod.tfvars`).
- hostd had no way to trust hostdev's self-signed CA. `repose.host.apiCA`
  (PEM text, public material) is written to the store and passed as
  `--api-ca` by both `hostd.service` and `repose-register.service`.
- `repose-host-net` used `jq -e` on `host_ca_pub` and the WireGuard fields,
  which the api fills and hostdev does not, so registration by hostdev
  left the bridge unconfigured. It now renders `wg0.conf` only when every
  WireGuard field is present (the unit is already conditioned on the file),
  writes an empty CA file otherwise, and says so in the journal.
- The generic `host` attribute has bootstrap sshd off, and until workstream
  06 puts hosts on WireGuard the installer, the join-token delivery and
  operators reach a host only over the VNet through the edge, which the
  nftables `input` chain admits only while `repose.host.bootstrap.enable`
  is on. So a production host is its own attribute, `nixosConfigurations.
  host-<name>` from `nix/hosts/<name>.nix`, selected by the new root
  variable `host_flake_attrs` in `infra/azure/{prod,staging}`; `host-01`
  sets the edge address, hostdev's CA, the operator key for bootstrap and
  the Blob endpoint and identity. *Rejected:* setting these on the generic
  `host` (every future host would trust a dev CA and expose bootstrap
  sshd); passing them at install time (nixos-anywhere takes a flake
  attribute, nothing else). Interfaces: `host-conventions.md` (the
  `hostd.service` command line). The edge firewall also opens 443, which
  the NSG already did, for hostdev now and the preview-proxy stub later.

**I-41. The data disk is found at install time, not named in advance.**
(m1 integration, 2026-09-20) I-39 named the data disk `/dev/nvme0n2` "LUN
0", while infra attaches it at LUN 10 and Azure's remote-NVMe FAQ says v7
sizes put cached disks (the OS disk) on one controller and uncached data
disks on a second one, which would make a Premium SSD v2 data disk
`/dev/nvme1n1`. Neither name has been seen on a real v7 host. The disko
layout's Azure hook, which already runs before the data disk is touched,
now resolves `/dev/disk/repose/data` itself: the SCSI by-LUN path when the
size is SCSI, otherwise the single NVMe disk that is not the OS disk, and
it fails with the list of disks it saw when there is not exactly one.
`repose.host.dataDevice` defaults to that symlink and the post-install check
asserts the thin pool `/dev/vg-guests/thin` exists rather than a device
name. *Rejected:* fixing a namespace number (wrong on one of the two
controller layouts, and wrong again if the LUN changes); a udev rule in the
installer (nixos-anywhere's kexec image takes none). *Revisit when:* a host
has more than one data disk.


**I-42. The api's implementation shape: phased ops driven by one replica,
/internal on the gRPC app, CA material in the secrets table, guest host
certificates re-signed once the address is known.** (05, 2026-09-20)
Recorded where the code had to choose beyond what the docs said, or where
a doc disagreed with a contract:

- *Ops.* Every long operation is an `ops` row with a list of phases fixed
  at enqueue (`create` = build, create_guest; `destroy` = final snapshot,
  destroy_guest; `restore` = destroy old guest, build if no closure,
  restore, start; ...). Each phase is one hostd command whose
  `command_id` is written to the row before it is sent and whose result
  lands in `command_result`; the command is rebuilt from the database and
  re-sent with the same id after an api restart or on the host's next
  `Hello`, so nothing but the row has to survive. The driver runs on the
  replica holding advisory lock `LockOps`; a second replica serves HTTP and
  the stream but would need the internal forwarding hop 05 §5.1 describes
  before it can drive ops for hosts connected to it. *Rejected:* a
  goroutine per op (lost on restart); storing the serialised command
  (secrets in plaintext in a fourth place).
- *The gRPC app also serves `/internal`.* Coolify's proxy terminates TLS
  for the HTTP app, so it cannot require the gateway's client
  certificate. `/internal/*` is served by the `api-grpc` process on
  `API_INTERNAL_LISTEN` (8444) over HTTPS with
  `RequireAndVerifyClientCert` against the platform's X.509 host CA, the
  same authority that signs host certificates at `Register`; `repose-admin
  ca sign-client --name gateway` issues the client certificate. Gateway
  session reports are persisted in `gateway_sessions` so the HTTP app can
  show them.
- *CA material lives in the secrets table.* The two SSH CAs and the X.509
  host CA (certificate and key) are rows of the platform pseudo-project
  (`00000000-0000-7000-8000-000000000000`, owned by the pseudo-user
  `repose-platform`), envelope-encrypted like any secret, created by
  `repose-admin ca init` and loaded at start. The `HOST_CA_CERT` and
  `HOST_CA_KEY` variables 05 §5.14 listed are gone; `GRPC_SERVER_CERT/KEY`
  remain for the public listener. *Rejected:* PEM files in Coolify
  secrets (a fourth home for a signing key, and no rotation path).
- *Guest sshd keys are reserved secrets.* The key I-3 says the api
  generates is stored under the reserved names of I-10 in the project's
  secrets rows, so every `StartGuest` and `Restore` delivers the same key.
  At `CreateGuest` the host certificate can only carry
  `<slug>.<handle>`: the guest's address is assigned by hostd and comes
  back in the result. The api then re-signs the certificate with both
  principals for the next start. The gateway therefore verifies a guest's
  host key with `<slug>.<handle>` as the expected principal, which it
  knows from the login name, not with the address. Interface:
  `ssh-gateway.md`.
- *Samples are inserted, not copied.* `Samples` messages are written with
  `insert ... on conflict do nothing` in one batch per message rather than
  the single `COPY` of 05 §5.4, because hostd re-sends buffered samples
  after a reconnect and a duplicate primary key would fail the whole
  `COPY`.
- *Schema additions.* `hosts` gained `name`, the heartbeat columns and the
  join-token hash; `ops` gained phases, `params`, `command_result`,
  `result`, `revision_id`, `snapshot_id`, `audit_id`, `reboot_required`;
  `config_revisions` gained `kernel_changed` and `reboot_required`;
  `events` gained `tmux_window` (`window` is reserved in SQL),
  `ts_second`, `source`, `skew_seconds`, `host_event_id`, and its dedupe
  index applies only to hook kinds so state changes may repeat within a
  second; `snapshots` gained `restoring_op_id` (the guard the expiry job
  respects); `usage_hours` gained the three cost parts and the storage
  remainder the cap rule needs (shared with 09); new tables
  `events_outbox` (13's shape), `host_sessions`, `gateway_sessions`,
  `settings`. `db-schema.md` is the reference.
- *Consumed packages that do not exist yet.* `internal/billing` carries the
  price table and a `Disabled` pusher and portal (`503 billing_disabled`,
  I-16); the notification senders live in `internal/api/notify` with the
  documented headers; `internal/nixmenu` is the api's view of the catalog
  with a package-only menu (12 owns the contents and the service
  snippets). When 09, 12 and 13 land, the api swaps the implementation
  behind the same interfaces.
- *Small contract points.* `POST /projects` validates the name as 05 §5.3
  says (`[A-Za-z0-9._-]{1,64}`; a space is refused rather than slugged as
  `features/projects.md` suggests), and refuses with `payment_required
  {reason: card_required}` when the user has no card rather than creating
  a row that can never boot. `GET /logs?kind=console` returns the console
  excerpts hostd attaches to failed ops; the full console is in Loki. The
  SSE route's `data:` is `{seq, line}` JSON and `POST /internal/sessions`
  takes `{project_id, event: opened|closed, cert_serial}` (`api.md`).
  `repose-admin` talks to Postgres directly and has no `login`; the
  runbook's `repose-admin login` line is withdrawn. Rate-limit buckets are
  per replica.
**I-43. The fragment contract is enforced by a NixOS module,
`nix/guest/contract.nix`: `repose.overlays` from a pre-pass, `repose.system`
through a static allowlist, and one class-independent closure.** (12)
`workstreams/12-nix-config-pipeline.md` sketched `composeGuest { baseRef,
fragment, menuSnippet, guestParams }` with the menu's NixOS snippet as a
separate module. The wire carries one file (`Build.fragment`, I-28), so
the snippet travels inside the fragment as `repose.system = [ { ... } ]`,
a list of plain attribute sets whose first two levels must be in
`nix/guest/system-allowlist.json`; the composer defines each allowlisted
path statically and refuses anything else through an assertion naming the
option. A hand-written fragment may use the same door under the same list,
which is what makes the boundary real whatever produced the file. Two
things the sketch could not have known: home-manager's module list itself
needs `pkgs`, so `nixpkgs.overlays` cannot be read back from the evaluated
home-manager configuration (infinite recursion); `repose.overlays` is
instead read off the fragment in a pre-pass that calls a function fragment
once with the platform's own `pkgs` and `lib`, and may use nothing else.
And `Build` carries no class, so the browser slice's ceiling is a
percentage of guest memory (37.5 percent: 1.5/3/6 GB) instead of a
per-class constant, which makes the closure serve any class (I-34). Errors
are attributed to `fragment.nix` because `compose.nix` hands the path to
home-manager unimported; the contract module is not called `fragment.nix`
so the error mapping's `fragment.nix:L:C` can only mean the user's file.
`composeGuest { fragment | fragmentPath, class, baseVersion, guestd, hook,
extraModules }` replaces the sketch's signature; `guestSystem` is
`composeGuest { fragmentPath = "${fragment}/fragment.nix"; }`. *Rejected:*
trusting the api to be the only producer of `repose.system` (nothing
distinguishes its file from a user's); overlays as a home-manager option
(recursion); a second `Build` field for the snippet (a second file, a
second override input, and the takeover flow would have two things to
copy). Interfaces: `nix-build-contract.md`, `guest-conventions.md`
(browser slice), `features/config.md` "Writing a fragment".

**I-44. The menu package is `internal/menu`; `GET /catalog` carries `kind`
and `options`.** (12, for 05 and 08) `05-control-plane-api.md` named it
`internal/nixmenu`; the workstream that owns it (12) names it
`internal/menu`, and 05's text is corrected. The catalog is a YAML file
embedded in the package; `Load` validates it and lints every `nixos`
snippet against the allowlist, so a catalog entry outside the list fails
`go test`, not a build on a host. `api.md`'s `[{id, label, group,
description}]` gains `kind` and `options` (enum id, values, default),
which the dashboard needs to render a select; the old fields keep their
meaning. A generated fragment's second line, `# repose-menu: <json>`, is
the selection, so a menu-managed project round-trips without a second
store. Playwright MCP stays nixpkgs's (02's coupling to
`playwright-driver`), so `versions.json` does not list it.

**I-45. Fragment evaluation and builds run as `nixbuild` inside a
transient scope, against a `git+file://` flake, with `allowed-uris`
derived from the base checkout's lock file, and `--show-trace`.** (12, 03)
Four things the contract as written could not do, found by running it:
`path:<checkout>/nix` copies only `nix/` into the store, and
`nix/packages.nix` builds guestd from `../.`, so the flake must be named
`git+file://<checkout>?dir=nix` (the checkout is a git clone anyway);
restricted mode refuses the locked inputs the flake machinery fetches
during evaluation unless each exact URI (`github:owner/repo/rev?narHash=`)
is in `allowed-uris`, so hostd derives that list from `flake.lock` and adds
the fragment's directory, which admits those trees and nothing a fragment
can name; a truncated trace loses the fragment's line and column for
errors raised inside the module system, so the eval passes `--show-trace`
and the mapping takes the innermost `fragment.nix:L:C`; and `systemd-run
--scope` cannot switch user, so hostd runs `setpriv` to `nixbuild` (all
capabilities dropped, no new privileges) inside the scope, with
`RuntimeMaxSec` as a backstop 30 s past `timeout`. The messages are the
workstream doc's exact first lines (`syntax error at fragment.nix:L:C,
...`, `build timed out after 30 minutes while building X`, `closure is
31.2 GB, limit is 20 GB; largest paths:`); the doc's `eval_timeout` code
is the interface's `eval_failed` with "evaluation exceeded 60 s", because
`grpc-hostd.md`'s enum is what the api and CLI switch on. `nixbuild`
exists on every host (`nix/hosts/hostd.nix`), `/var/lib/repose/builds` is
0711, and the platform cache is a host option (`repose.host.overlayCache`)
passed as `--substituters`, not a constant in hostd. *Rejected:* keeping
`internal/nixbuild` as a second package next to 03's
`internal/hostd/nixbuild` (one implementation of one contract; 03's
package is extended in place, and the fixtures stay where the contract
says).

**I-46. The agent overlay is built from upstream release binaries pinned
in `versions.json` and cached on Cachix.** (12) Claude Code from
Anthropic's release bucket (the ELF the npm installer fetches), opencode
and pi from their GitHub release tarballs (bun-compiled, dynamically
linked, `autoPatchelfHook`), Codex from its static musl tarball, Gemini
CLI from the npm registry (a single bundle with no dependencies, run with
the guest's node). `scripts/bump-agents.sh` reads each upstream's latest,
prefetches, rewrites `versions.json`, builds, runs `--version`, and with
`--pr` opens the pull request; `.github/workflows/bump-agents.yml` runs it
daily. The binary cache is the Cachix cache `repose`
(`https://repose.cachix.org`): CI pushes the seven overlay packages on
every push to `main` when `CACHIX_AUTH_TOKEN` is set, and hosts add it
through `repose.host.overlayCache` once the owner has created the cache
and pasted its public key (`ops/AZURE-SETUP.md` step 16). *Rejected:*
nixpkgs as the source (R3-19; it also now marks `gemini-cli` for removal,
which is Google's tiering change, not a reason to drop an agent that works
with an API key); an S3 bucket served by `nix-serve` on the Coolify VM (a
service to run and a signing key to keep, for a cache of public
binaries); R2 through Nix's S3 support (works, but is a second credential
in CI for no gain until Cachix's free tier is outgrown). *Revisit when:*
the cache passes 5 GB or a private overlay package appears.

**I-47. Base bumps are a planner and a runner in `internal/basebump` over
two interfaces the api implements.** (12, for 05) The api does not exist
yet, so the policy is a package with `NewPlan` (which projects a version
reaches: not held, last build not failed, running or stopped, not already
on it; spread over 24 h, 2 h for `--security`, two at a time per host),
`Runner.Run` (build, then `ApplyConfig` for a running guest; a stopped
guest is `built` and boots the closure at its next start; `kernel_changed`
ends as `needs_reboot` with the `base_update_ready` event; any failure is
`failed` with `base_update_failed` and the project keeps its base),
`Summarize` for `repose-admin base status`, `Rollback` for `base rollback`
and `StatusLine` for the base part of `repose status`. Workstream 05 wires
`Dispatcher` and `Recorder` to Postgres and the stream and schedules the
run from `base publish`. The checklist's "three projects" evidence is the
package's test until the api exists.
**I-48. virtiofsd's sandbox is `namespace`, and hostd attaches taps with
exactly the host-conventions sequence.** (14, review of 01 and 03,
2026-09-20) Two places where merged code disagreed with the merged
contract, found by reading them side by side:

- `internal/hostd/virtiofs` started virtiofsd as user `virtiofsd` with
  `--sandbox chroot`. chroot(2) needs CAP_SYS_CHROOT, and virtiofsd 1.14.0
  refuses the combination outright: `Error entering sandbox: sandbox mode
  'chroot' can only be used by root (Use '--sandbox namespace' instead)`
  (reproduced on the dev box, exit 1). Every guest create would have failed
  at step 8 on a real host, and the tempting "fix" of dropping `User=`
  would have put a root virtiofsd with the whole store in front of every
  tenant. Namespace mode is what `03-hostd.md` §5.5 and the runner's
  `bin/virtiofsd` already said; `host-conventions.md` and `01-host-nixos.md`
  said chroot and now say namespace. The host enables unprivileged user
  namespaces (`security.allowUserNamespaces`, `kernel.nix`) for this.
  *Rejected:* `AmbientCapabilities=CAP_SYS_CHROOT` on the unit (a
  capability on a process that faces tenant-controlled FUSE traffic, to
  keep a mode whose only advantage is not needing user namespaces).
  *Verify on the first host:* `systemctl status virtiofsd@<guest>` is
  active and `ls /nix/store` works in the guest; the dev box cannot run
  namespace mode itself (Ubuntu's `apparmor_restrict_unprivileged_userns`).
- `internal/hostd/net` created taps with `ip tuntap add ... mode tap` and
  attached them with a bare `ip link set master`, then added the guest to
  a set `inet repose guests { ip . tap }` that no host declares: I-18
  moved admission into the `bridge repose` table with type `ether_addr .
  ipv4_addr . ifname`, and 03's code predates that. On a host, `nft add
  element` fails and every create stops at step 6; had the set been
  declared to make it pass, taps without `learning off` and a static FDB
  entry would let a guest claim another guest's MAC and receive its
  inbound frames (the bridge learns before the nftables input hook
  drops). The `Net` interface now carries the MAC and the golden test is
  the command list from `host-conventions.md` "Network", verbatim.
  Interface text unchanged; the code follows the doc.








**I-49. The tmux-idle heuristic never reads pane content, and its metrics
carry the `repose_api_*` prefix, not `repose_notify_*`.** (13, review of 04
and 05) Two places where code merged ahead of this workstream disagreed
with `13-notifications.md` as written, found by auditing 04 and 05's
already-built pipeline against it:

- §5.4 described a `needs_input` heuristic that runs `tmux capture-pane`
  and matches prompt patterns (`❯`, `[y/N]`, ...) against the pane's last
  three lines. `internal/guestd/sample` (04) never captures pane text at
  all: `needs_input` comes only from a real hook (`RecordHook`), and the
  heuristic's only signal is whether the pane's process tree has consumed
  CPU since the last refresh, per `docs/workstreams/04-guestd.md` §5 and
  `docs/features/agents.md` (already correct). This is strictly *more*
  private than the documented design (there is no `patterns.go`, no
  `capture-pane` call to grep for, so the checklist's "pane contents are
  never logged, stored or sent" item is satisfied by construction rather
  than by discipline), and it was the right call: a hookless agent's last
  line is exactly the terminal content §5.1 says a summary must never carry
  beyond what a hook payload itself gives, and a heuristic has no hook
  payload. *Rejected:* implementing capture-pane matching to match the
  original doc (adds the exact surface area the privacy boundary exists to
  avoid, for a `needs_input` signal only three of five agents lack, and
  those three already get it from `RecordHook` once they gain a hook).
  §5.4 below is rewritten to describe the built heuristic (CPU-busy,
  90-second quiet window for hookless agents from `features/agents.md`,
  not the 30/30 split originally written); the `needs_input` row is
  removed from the heuristic's state table because no code path produces it
  outside a real hook.
- 05 gave every api metric the `repose_api_` prefix for one family per
  component (`repose_api_notify_total`, `repose_api_outbox_depth`, ...),
  not the bare `repose_notify_*` names §5.8 listed, and `EventsTotal`
  carries `{kind}` only, with `agent` and `source=hook|heuristic` never
  added (the heuristic's synthetic completions and a real hook's are the
  same `kind` in the same table; splitting them needed a label 05 had no
  reason to add before this workstream existed). This workstream adds
  `repose_api_notify_delivery_latency_seconds` (a histogram of event ts to
  delivered ts, the one 5.8 metric with no equivalent) and leaves the
  `repose_api_*` convention alone rather than renaming a dozen already-
  deployed families for one workstream's original wording: a consistent
  per-component prefix is worth more than matching a name picked before
  the component existed. §5.8 is rewritten to the real names.

Also closed here, because the pipeline existed but the specific behaviour
did not: the unsubscribe link (`GET /v1/notify/unsubscribe?token=`, a
non-expiring HMAC-signed user id, keyed by a secret auto-provisioned into
the platform pseudo-project the first time the api starts — an operator
step here, unlike `repose-admin ca init`, would leave the very first
account's unsubscribe link broken until someone remembered to run it);
`billing_stopped`'s dedicated subject line; and `host_moved` /
`snapshot_failed` actually reaching `events` (the restore result handler
and `onFail`'s `snapshot` case previously only logged or set
`projects.last_error`). `ops.Engine` gained an `EventSink` interface
(satisfied by `events.Ingest.Platform`, nil in the admin CLI's ad-hoc
engine) for this. `billing_stopped` still has no producer: workstream 09
is the one that will call `events.Ingest.Platform` for it. Interfaces:
`api.md` (`/notify/unsubscribe`).

**I-50. Gemini CLI and pi both gained hook mechanisms since 5.3's "at time
of writing" rows were written; Gemini CLI itself stopped serving
individual-tier requests on 2026-06-18.** (13, 2026-09-20) 5.3 and the
checklist require resolving "at time of writing" rows before calling this
workstream done. Checking now, against the agents' own current docs:

- **Gemini CLI** ships a hook system (`geminicli.com/docs/hooks/`,
  `google-gemini/gemini-cli` `docs/hooks/reference.md`) including a
  `Notification` hook (fires on idle, awaiting-input and tool-confirmation,
  which is exactly `needs_input`) and a post-agent-loop hook usable as
  `completed`. The tmux-idle heuristic this workstream ships for Gemini is
  therefore not "the mechanism" any more, just the fallback for a version
  where hooks are absent or the platform has not wired them.
- More urgently: Google stopped serving `gemini-cli` requests for free,
  Pro and Ultra tier accounts on 2026-06-18, replacing it with a separate,
  closed-source binary, Antigravity CLI (Google's own developer blog,
  "Transitioning Gemini CLI to Antigravity CLI"; enterprise accounts with a
  Code Assist license or a bare API key are unaffected). `nix/overlay/agents`
  still packages `gemini-cli` (I-46); for any user without an API key or an
  enterprise license, the agent DESIGN.md lists as one of five now fails to
  authenticate at all, hook or no hook. This is a product decision beyond
  this workstream's remit (DESIGN.md §11, R2-11's agent list, and 02/12's
  packaging), not something to silently patch here.
- **pi** (`earendil-works/pi`, the coding agent this platform ships) has a
  real hooks directory, `~/.pi/agent/hooks/`, with `onStop` and
  `ctx.ui.notify()`. The "its hooks if present in the shipped version"
  branch of 5.3's row is therefore live, not hypothetical.

None of this is implemented here: mapping Gemini's and pi's actual hook
JSON into `{agent, kind, summary}` needs the real binaries to verify wire
shapes against (the fixture-per-shape discipline `internal/guestd/hooks/
testdata` already follows), which this session does not have, and 04's
already-reviewed `internal/guestd/hooks` package is not this workstream's
to extend blind from search-engine snippets — a wrong mapping silently
drops or mis-labels every Gemini and pi notification, which is worse than
the honest heuristic currently in place. *Rejected:* implementing the
mapping now from documentation alone (no way to verify against a real
payload before shipping); leaving 5.3's "at time of writing" wording
unresolved (the checklist item exists precisely so this gets checked and
written down, whichever way it comes out). The heuristic stays as the
current, working mechanism for both agents; `docs/features/agents.md` and
`13-notifications.md` §5.3 are annotated to point here rather than
rewritten to describe an unverified mapping. **The owner should decide
whether Gemini CLI stays in the agent list at all**, given it no longer
authenticates for the tier most users are expected to be on.


**I-51. `guest@<id>` runs Cloud Hypervisor as the `hostd` user inside a
systemd sandbox; hostd itself stays root.** (14 follow-up, review H-2,
2026-09-20) `docs/SECURITY.md` accepted that a KVM escape lands in the
Azure VM; as built it landed as root, which is every tenant on the host,
the host's mTLS identity and its Blob credential. The transient unit now
carries `User=hostd` and the property list pinned by
`internal/hostd/guest/testdata/unit.golden`: `NoNewPrivileges`, an empty
`CapabilityBoundingSet`, `ProtectSystem=strict`, `ProtectHome`,
`PrivateTmp`, the `ProtectKernel*`/`ProtectControlGroups`/`ProtectProc`
set, `RestrictNamespaces`, `RestrictRealtime`, `RestrictSUIDSGID`,
`LockPersonality`, `SystemCallArchitectures=native`, `DevicePolicy=closed`
with `DeviceAllow` for `/dev/kvm`, `/dev/net/tun` and the guest's own
`/dev/vg-guests/g-<id>` only, `RestrictAddressFamilies=AF_UNIX AF_VSOCK`
(Cloud Hypervisor's tap ioctls use AF_UNIX sockets; AF_INET is needed only
for `--net ip=`, which hostd never passes), and `TemporaryFileSystem=
/var/lib/repose/guests` with `BindPaths=` of the guest's own directory, so
one hypervisor cannot see, let alone connect to, another guest's
`vsock.sock` (a direct line to that guest's guestd). `--seccomp true` is
written out on the argv. What the host provides for it: `hostd` in group
`kvm`; a udev rule making `dm-*` nodes with `DM_VG_NAME=vg-guests` and
`DM_LV_NAME=g-*` group `hostd` mode 0660 (snapshots and the pool stay
`root:disk`); `/var/lib/repose/guests` 0711 with each guest directory
`1770 root:hostd` (the sticky bit keeps `ch.args`, `guest.json` and
`console.log` out of the hypervisor's reach) and a `virtiofsd/`
subdirectory `0750 virtiofsd:hostd` where virtiofsd binds
`virtiofsd.sock` with `--socket-group hostd`. The socket path moved from
`<dir>/virtiofsd.sock` to `<dir>/virtiofsd/virtiofsd.sock`; nothing
outside hostd read the old path. hostd remains root (LVM, nftables, taps)
and connects to the guest's sockets with root's override.
*Rejected:* one system user per guest (`DynamicUser=`): the cleanest
separation, but taps and volumes need a known owner before the unit
exists, and the shared-uid gap it would close is already narrowed by
`DeviceAllow` and the private guests directory; recorded as review L-11
for a later pass. `AmbientCapabilities=` of any kind: Cloud Hypervisor
needs none with `kvm` group access. Dropping `ProtectSystem=strict`
because the store is on `/`: the store is read-only for the unit either
way and the unit reads only the closure's kernel and initrd.
*Verify on the first host:* `systemctl show guest@<id> -p User` is
`hostd`; `ps -o user= -p $(systemctl show -p MainPID --value guest@<id>)`
is `hostd`; `ls -l /dev/mapper/vg--guests-g--*` is `root hostd`; the
guest boots and `ls /nix/store` works inside it; `test/isolation`
`TestHypervisorRunsAsHostdUser`.
**I-52. The `obs` package fixes what §5 left to call sites, and its
component and label lists are wider than §5's by two and three.** (10)
`internal/obs` is the only place a repose binary gets a logger or a metrics
registry, and both constructors enforce the rules rather than documenting
them: the log handler adds `component` (so no call site can omit it) and
redacts the never-log field names; the registry refuses a metric outside the
`repose_` namespace or with a label outside the low-cardinality list, at
registration, so a bad name stops the binary at startup. Three list changes
were needed to describe what exists:

- Components gain `hostdev` and `hook`. §5 names six
  (`api|hostd|guestd|gateway|cli|admin`), but `cmd/hostdev` (I-17) and
  `cmd/repose-hook` (04) are separate binaries, and a Loki query that cannot
  tell hostd from the api stand-in it talks to is not worth running.
  *Rejected:* logging hostdev as `api` (its lines would be mixed with the
  real api's in the same query for the rest of the project's life).
- Metric labels are §5's list (`component, host_id, class, state, kind,
  reason, route, status`) plus the ones §5's own families use (`result`,
  `direction`, `method`, `channel`), plus `version` for `repose_build_info`
  and `phase` for I-54. §5's prose list was incomplete against its own
  tables; the tables are the law.
- `repose_host_guestd_unreachable{guest_id}` is deleted. It broke the rule
  in the same section that defined it ("`project_id` and `guest_id` are
  never labels in Prometheus"); `repose_host_guestd_lost`, the count, is the
  metric, and which guest it is comes from the `guestd_lost` log line and
  the `Warning` the api receives.

The redaction floor is docs/ops/OBSERVABILITY.md's six names plus the
never-log entries that have an obvious field name (`email`, `handle`,
`remote_url`, `prompt`, `args`, `argv`, `env`, `cmdline`, `command_line`,
`user_agent`), matched exactly rather than by prefix so that `cert_serial`,
`key_id` and `token_used` survive. Guests import `obs` for the logger only
and pay 2.5 MB of binary for the metrics and trace code that comes with one
package (19.8 MB to 22.3 MB, measured); §2 asks for one package and 2.5 MB
in a guest with a 20 GB volume is not a reason to split it.

**I-53. An operator's `Exec` argv is not logged, only its `audit_id` and
length.** (10, amends 03) hostd logged `argv` on the audited-exec path with a
comment saying it was the one place that was allowed. It is not: process
arguments are on the never-log list without an exception for operators, and
the audit trail that must carry the command is the api's `audit_log` row
keyed by the same `audit_id` (`db-schema.md`: "every Exec"). A Loki reader
with the Grafana password is not the same audience as an auditor with
Postgres access. *Rejected:* keeping it with a redaction filter (the argv of
`repose-admin exec -- cat /home/dev/app/.env` is exactly what the list
forbids, whoever typed it).

**I-54. `meter_samples` carries `guestd_ok`.** (10 and 05, independently)
§5's day-one signals and `grpc-hostd.md`'s `GuestSignals` both carry
`guestd_ok`, the api receives it on every sample, and `db-schema.md` had
nowhere to put it, so the per-guest dashboard could not show the gaps where a
guest's signals are unknown rather than zero. Workstream 10 proposed the
column and workstream 05 had already added it by the time the two merged; the
shape kept is 05's, `guestd_ok bool` with no default, because a sample from
before the column existed is honestly null rather than optimistically true.
The Per-guest resources dashboard therefore reads `guestd_ok is false`, not
`not guestd_ok`. *Rejected:* inferring it from null signals (a guest with no
sessions and a lost guestd would look the same, which is the distinction I-31
added the `partial` flag for). Interface: `db-schema.md`.

**I-55. Dashboards are generated from `ops/dashboards/gen.py` and the JSON is
committed; `ops/` is laid out as §2 says, not as PROMPTS.md says.** (10) A
Grafana dashboard is 400 lines of JSON of which four matter, and the same
panel shape appears thirty times; seven hand-maintained files drift.
`gen.py` is the source of truth, the JSON next to it is committed because
Grafana provisioning reads files, and `ops/check.sh` fails when they
disagree — the arrangement of I-38. `docs/workstreams/PROMPTS.md` says
"dashboards and alert rules under `ops/grafana/`" while the workstream doc
§2 says `ops/dashboards/` and `ops/alerts.yaml`; the workstream doc wins and
`ops/grafana/` holds Grafana's provisioning files only. *Rejected:* writing
the JSON by hand (the first panel rename proves why); keeping the generator
out of the repository and committing only its output (nobody can then
regenerate it).

**I-56. Two alerts beyond §5's eleven, and the Fluent Bit metrics port is
open on wg0.** (10) §6 describes two failures whose only symptom is silence:
Loki unreachable from a host (Fluent Bit buffers and retries forever) and
Prometheus unable to scrape a host (metering continues over the gRPC stream,
so nothing else complains). `FluentBitStuck` and `HostScrapeDown` are those,
with runbook headings of their own. Seeing the first needs Fluent Bit's own
metrics, so it serves them on the WireGuard address
(`repose.host.observability.fluentBitMetricsPort`, default 2021) and the
nftables `input` chain admits that port from `wg0` alongside 22, 9100 and
9101. *Rejected:* scraping Fluent Bit through node_exporter's textfile
collector (a shipper's health reported by a cron job that writes a file the
shipper's failure does not affect); leaving it unmonitored (a host stops
shipping logs and nobody knows until they go looking for a line that is not
there). Interface: `host-conventions.md`.

**I-57. `repose_host_build_phase_duration_seconds{phase}` splits eval from
build.** (10, for 03 and 12) §5's Builds dashboard asks for "eval vs build
time" and nothing measured either: `nixbuild.Build` runs `nix eval` and then
`nix build` and timed only the pair. The two have different caps (60 s and
30 minutes, R5-4) and different causes — a slow eval is the fragment, a slow
build is a substituter or a source build — so a single number cannot answer
the question the panel asks. `nixbuild.Result` now carries both durations,
the manager observes them, and `build_done` logs `eval_ms` and `build_ms`.
*Rejected:* parsing the phase out of the build log (the log is the tenant's
Nix output, not a metric source).

**I-58. `repose-hook` reads `REPOSE_HOOK_AGENT`, the name the wrappers
export.** (10, fixing 02 and 04) The Go binary of workstream 04 read
`REPOSE_AGENT`; the wrappers of workstream 02
(`nix/overlay/agents/wrap.nix`) export `REPOSE_HOOK_AGENT`, which is also
what `docs/interfaces/guest-conventions.md` documents; and `nix/flake.nix`
ships the Go binary in every guest. So every agent hook in every guest read
an empty agent name and exited without posting: no `agent_event`, no
notification, and nothing in any log to say so. The guest-base VM test found
it by waiting 15 minutes for a hook that could never arrive.

The binary now prefers `REPOSE_HOOK_AGENT` and keeps `REPOSE_AGENT` for one
release, and accepts the socket under both `REPOSE_HOOK_SOCKET` (its own
name) and `REPOSE_HOOKS_SOCKET` (the shell implementation's). *Rejected:*
changing the wrappers instead (the interface doc names the variable, and a
wrapper is what a user's own agent config may already set); keeping two names
permanently (a second name for the same thing is how a grep misses half the
uses). *Why this workstream:* `agent_event` is one of the events
docs/workstreams/10-observability.md §5 requires guestd to emit, and it could
not fire. Interface: `guest-conventions.md`.

**I-59. `internal/obs` is three packages, because a guest pays for what it
imports.** (10, amends I-49) §2 asks for "one Go package used by every
binary", and one package it was until the guestd VM test failed on
`docs/workstreams/04-guestd.md` §7's budget: guestd's resident memory came to
20.2 MB against a 20 MB limit, because importing `obs` for the logger linked
in the Prometheus client and the OpenTelemetry SDK with their package
initialisers. guestd has no metrics endpoint and no traces of its own — it
speaks vsock and nothing else — so it was paying 2.5 MB of binary and 700 KB
of RSS for code it cannot reach.

The split follows the dependency weight: `internal/obs` is logs and names
(stdlib only: the logger, the component and event lists, the request-id
context helpers), `internal/obs/metrics` is the Prometheus registry and the
api and gateway families, `internal/obs/instrument` is the OpenTelemetry
setup, the gRPC options and the api's HTTP middleware. The rules are enforced
in the same places as before, and the naming test covers all three. guestd now
links neither heavy dependency: `go list -deps ./cmd/guestd | grep -cE
'prometheus|opentelemetry'` is 0, and the stripped binary is 13.7 MB.
*Rejected:* raising 04's budget (the budget exists because guestd competes
with the agent for two vCPUs and 4 GB, and it was right); keeping one package
and hoping the linker drops the unused half (package initialisers are always
kept, which is what the measurement showed); a build tag (a binary whose
behaviour depends on how it was built is worse than a package boundary).

**I-60. One observability package, one tracing setup, one api metric family.**
(merge of 10 with wave two, 2026-09-20) Workstreams 05 and 10 each built what
they needed while the other was unmerged, so the merge found three pairs:

- `internal/obs`. 05's version said in its own header "Workstream 10 owns the
  naming rules; this is the subset the api needs", so 10's is the package and
  05's is gone. Two things of theirs were better and were kept: redaction
  matches a *substring* of the field name, because a call site writes
  `access_token` and `user_email` rather than the bare word, with an exact
  allowlist for the ids and counts that contain one (`cert_serial`,
  `key_version`, `secrets_count`, and 10's own `argv_len` and
  `summary_bytes`); and `WithLogger`/`Logger(ctx, fallback)`, which is how the
  api gives every handler the request's fields.
- `internal/otel` and `internal/obs/instrument`. Both installed the OTLP
  exporters and the noop provider. 05's had the bug 10's on-path test caught
  the day before: `resource.Merge` of `resource.Default()` (schema 1.43.0)
  with a `semconv/v1.26.0` resource returns "conflicting Schema URL", so the
  api would have refused to start the moment anyone set
  `OTEL_EXPORTER_OTLP_ENDPOINT`. `internal/otel` is gone; `SetupTracing`
  gained the standard `OTEL_EXPORTER_OTLP_PROTOCOL` switch so 05's OTLP/HTTP
  deployment and 10's gRPC one both work, and a test covers each. The OTLP
  *metric* exporter 05 also installed is not replaced: DESIGN §15 and §5 make
  Prometheus the metrics path, and pushing metrics to a collector nobody runs
  is weight without a reader.
- The api metric family. 05 implemented every name §5 lists (and more) in
  `internal/api/metrics`; 10's `APIMetrics` is gone, and
  `families_api_test.go` holds 05's to §5 the way `families_host_test.go`
  holds hostd's. The api's registry now comes from `internal/obs/metrics`, so
  all 26 of its series are checked for the namespace and the label list at
  startup — which is how `repose_api_secrets_ops_total{op}` was noticed and
  `op` added to the list as the bounded enum it is. `HTTPMiddleware` went the
  same way: 05's server already emits the `request` event with a request id.

Three gaps the same check found in 05's code, fixed here: the no-capacity
placement path logged nothing and counted nothing (`schedule_fail` and
`repose_api_schedule_total{result}`), the partition maintenance failure had no
series for the `PartitionDropFail` alert to read, and `admin_action` had no
producer — it moves from the api's required events to the admin CLI's, because
05 built `repose-admin` against Postgres directly and the line belongs where
the `audit_log` row is written. `stripe_webhook` stays required of the api and
is listed in `obs.PendingEvents` as owed by workstream 09, which has no
webhook route yet.

*Why this workstream made the calls:* `docs/workstreams/README.md` gives 10
the metric and log naming, and a merge that keeps both of everything is how
`repose_api_*` ends up meaning two things. *Rejected:* keeping 05's obs and
deleting 10's (it has no component enum, no event registry, no source lint and
no metrics enforcement); keeping both tracing setups behind a flag.

**I-61. The store export bind is made private before `.links` is masked.**
(m1 integration, 2026-09-20) On host-01 every store write failed with
`Read-only file system` on `/nix/store/.links`: the tmpfs mask
`repose-store-export.service` mounts over `/run/repose/store-export/.links`
had propagated onto `/nix/store/.links`, because a bind mount joins its
source's peer group and NixOS mounts `/` shared. `nix copy` into the host,
`nix-store --optimise` and hostd's `Build` all write there. The unit now
runs `mount --make-private` on the export before the remount and the mask,
and the host-services VM test asserts `/nix/store/.links` is not a mount
point. *Rejected:* dropping the mask (the enumeration leak 01 §5 closes);
masking with a bind of an empty directory (propagates the same way).

**I-62. virtiofsd's socket lives in a subdirectory it owns, and hostd fails
step 8 when virtiofsd exits.** (m1 integration, 2026-09-20) The first
guest on host-01 died a minute after start with "guest did not become
ready": virtiofsd, which I-48 runs as the unprivileged `virtiofsd` user in
namespace sandbox mode, had exited at once because it could not create
`virtiofsd.sock` in the root-only guest directory (0750 under a 0700
parent), and Cloud Hypervisor retried the missing socket for 60 s. hostd
now creates `/var/lib/repose/guests/<id>/` as root 0710 with group
`virtiofsd` and `<id>/virtiofsd/` owned by that user, the socket is
`virtiofsd/virtiofsd.sock`, `/var/lib/repose/guests` is a 0710
root:virtiofsd tmpfiles directory instead of a `StateDirectory`, and step 8
waits up to 10 s for the socket while checking the unit, so a dead
virtiofsd fails the create as step 8 with its unit named. *Rejected:*
making the guest directory group-writable (virtiofsd could then rewrite
`ch.args`, which hostd hands to a root Cloud Hypervisor at the next start);
running virtiofsd as root in chroot mode (what I-48 moved away from);
socket activation through a transient socket unit (an fd-passing path
nothing else in hostd uses). Interface: `host-conventions.md` (guest
directory row and the CH invocation).

**I-63. The guest disk is passed to Cloud Hypervisor with
`image_type=raw`.** (m1 integration, 2026-09-20) With the type
auto-detected, Cloud Hypervisor 53 logs "Autodetected raw image type.
Disabling sector 0 writes" and rejects the guest's first write to sector 0;
ext4 keeps its primary superblock there, so `/sysroot` failed to mount with
`I/O error, dev vda, sector 0` and the initrd dropped to emergency mode
(console log of host-01's second guest). hostd names the type explicitly
in `ch.args`. Interface: `host-conventions.md` (the CH invocation).

**I-64. guestd binds its vsock listener to any CID.** (m1 integration,
2026-09-20) `internal/vsockrpc.Listen` bound `VMADDR_CID_HOST` (2), which a
guest kernel refuses with `cannot assign requested address`; guestd
restarted every two seconds and never sent `Ready`, so the first fully
booted guest on host-01 failed create at step 10. The dev-socket mode and
the QEMU VM test never exercise the vsock bind, which is why it survived
until a real host. The listener now binds `VMADDR_CID_ANY`.

**I-65. virtiofsd does not announce submounts.** (m1 integration,
2026-09-20) Inside the first running guest on host-01 every nix client got
`Connection reset by peer` and `journalctl -u nix-daemon` said `creating
directory "/nix/store/.links": Object is remote`. The store export masks
`.links` with a tmpfs (01 §5), virtiofsd 1.14 announces that mountpoint to
the guest by default, the guest kernel mounts it as its own virtiofs
submount under `/nix/.ro-store/.links`, and overlayfs refuses lookups that
cross a mount boundary inside a lower layer with EREMOTE. The nix daemon
creates `.links` at startup, so it died on every connection, which also
failed `home-manager-dev.service` at boot. hostd now passes
`--no-announce-submounts`; the guest sees an ordinary empty directory and
the enumeration leak stays closed. *Rejected:* dropping the mask (01 §5's
reason stands); a guest-side `nix.conf` workaround (the daemon creates the
directory unconditionally).

**I-66. `ResizeVolume` on a running guest calls Cloud Hypervisor's
`vm.resize-disk` between `lvextend` and `GrowFs`.** (m1 integration,
2026-09-20) On host-01 a resize from 40 to 60 GB returned ok, LVM showed
60 GB and guestd ran `resize2fs`, but the guest's `/dev/vda` still reported
40 GB: virtio-blk keeps the capacity the device was created with until the
hypervisor is told. hostd now calls `vm.resize-disk` on `_disk0` first; a
stopped guest picks the size up at its next boot as before.

**I-67. hostd registers the guest's closure in the guest's nix database:
`RegisterPaths` after `Ready`, and `registration` inside `Switch`.** (m1
integration, 2026-09-20) Inside the first running guest on host-01,
`nix path-info /run/current-system` said "is not valid": the guest's
database is created empty on its thin volume and the shared store puts
paths on disk without registering them. So `ApplyConfig` failed at
guestd's `nix-env --set` ("nix-env exited 1"), `home-manager-dev.service`
failed at every boot, and a user `nix` command touching a system path
would have tried to fetch it. The host has the metadata: hostd now sends
`nix-store --dump-db` of the closure (480 KB for the 6 GB base) as
`RegisterPaths` right after `Ready` and as the `registration` field of
every `Switch`; guestd runs `nix-store --load-db` (idempotent) and writes
`/run/repose/paths-registered`, which `repose-paths.service` waits for
before `home-manager-dev.service` runs. *Rejected:* a registration file in
the shared store named on the kernel command line (a store path that would
need its own GC root and a second delivery path); computing hashes in the
guest (`nix-store --load-db` needs the NAR hashes only the host has);
skipping `nix-env` in `Switch` (leaves the database wrong for every later
nix command). Interfaces: `vsock-guestd.md`, `guest-conventions.md`,
`proto/repose/guestd/v1/guestd.proto` (old shape accepted: an empty
registration means the previous behaviour).

**I-68. Reconcile removes `snap-*` volumes left by an interrupted
snapshot.** (m1 integration, 2026-09-20) `kill -9` of hostd on host-01
between the LVM snapshot and its removal, with a Blob upload in flight,
replayed the Snapshot command correctly after the restart (same
`command_id`, a fresh snapshot, result ok) but left
`snap-<guest>-<ts>` from the killed attempt in `vg-guests`, holding thin
pool space for ever. Reconcile at start now removes every `snap-*`
volume: hostd has no snapshot in flight when it starts, and a replayed
command makes its own. *Rejected:* naming the snapshot after the
`command_id` and reusing it on replay (an upload that died half way would
resume from a snapshot taken before the guest wrote more, which is
correct but the same as a fresh one, for extra state).

**I-69. The `virtiofsd` user is in group `hostd`.** (m1 integration,
2026-09-20) The first create on host-01 under the I-49 sandbox failed at
step 8: virtiofsd logged "`<guest dir>/virtiofsd` does not exist or is not
a directory" because the guest directory is `1770 root:hostd` and the
virtiofsd user was in no group but its own, so it could not traverse it;
and `--socket-group hostd` needs the same membership, since an
unprivileged process can only chgrp into a group it belongs to. The user
gains `extraGroups = [ "hostd" ]`. What that widens: virtiofsd can create
files in a guest directory before it sandboxes itself (the sticky bit
keeps it from removing hostd's, and `ch.args` is `0640 root`); it gains
nothing under the store export, which is what the user exists to protect.
*Rejected:* `1771` on the guest directory (does not fix the chgrp); a
socket directory under `/run` outside the guest directory (a second
layout for one file).

**I-70. The control plane is created now, with its Coolify pinned and its
dashboard off the network.** (11, owner, 2026-09-20) `coolify_count = 1` in
`prod.tfvars`. I-24 deferred the VM until "wave 3" on the grounds that it
would bill for nothing while the api did not exist; the api and `repose-admin`
are merged, so the trade has flipped. Three things were settled with it:

- **The Coolify release is pinned** (`coolify_version`, `4.3.23`) and
  `AUTOUPDATE=false`. The installer's own default is the moving `latest`, so
  the VM could not be rebuilt onto the version it had been running, and the
  thing that deploys the api could upgrade itself overnight. *Rejected:*
  tracking `latest` and pinning nothing (a rebuild after a loss is the moment
  a version surprise is least affordable).
- **Coolify's dashboard is not in the NSG.** It listens on 8000 over plain
  HTTP and, before an admin account exists, anyone who reaches it can create
  one. Operators reach it with `ssh -L 8000:127.0.0.1:8000`, on the port the
  NSG already opens to `operator_cidrs`, and the control subnet's NSG carries
  a `postcondition` that fails the plan if 8000, 6001, 6002 or `*` ever
  appears as an inbound rule. *Rejected:* opening 8000 to `operator_cidrs`
  (an unauthenticated admin panel on the internet for the length of one
  setup, and NSG lists are edited more often than they are re-read).
- **The apply waits for Coolify to be healthy.** `terraform_data.ready` reads
  the `coolify` container's Docker health status — the signal the installer
  itself waits on — and also fails when `rclone` or `pg_restore` is missing,
  because those are the first two commands of the runbook's restore
  procedure. *Rejected:* returning as soon as the VM boots (the operator
  cannot tell a machine still pulling images from one whose cloud-init died
  twelve minutes ago).

The cost re-query this forced corrected the estimate: the `D4s_v7` is
$0.265/h, not the $140.16 a month the 2026-09-19 table carried, so the control
plane is about $237 a month and the environment about $1,094, over the $1,000
budget alert (`ops/AZURE-SETUP.md` step 7). Setting `coolify_count` back to 0
destroys the VM, its OS disk, Postgres and every Coolify application
definition; the retention that matters is the R2 dump *and*
`/data/coolify/source/.env`, whose `APP_KEY` decrypts the credentials in that
dump. `ops/coolify.md` is the click path and holds that last point where it
will be read before a restore rather than after one.

**I-71. `manage_dns` defaults to false, and the absence of a record is not
the absence of an answer.** (11, 2026-09-20) The roots defaulted `manage_dns`
to true while no `CLOUDFLARE_API_TOKEN` existed, so every plan depended on the
local tfvars turning it off, and a plan without them failed inside the
Cloudflare provider with an authentication error naming neither the variable
nor the step that creates the token. It now defaults to false in both roots,
`infra/dns` validates the zone id where a null one would otherwise reach the
provider, and `make plan ENV=r2` fails on the missing token with the step that
creates it.

The reason this matters beyond tidiness was found by resolving the names on
2026-09-20: **`herakraft.co` answers every name under it from a proxied
wildcard record.** `ssh.repose.herakraft.co` therefore resolves today, to
Cloudflare's proxy, which carries neither SSH nor WireGuard — so a user
following the documented hostname, and any host configured with it as a
WireGuard endpoint, fails in a way that looks like a firewall problem. The
environment module raises it as a plan-time `check` warning whenever
`manage_dns` is false, `infra/README.md` has the four records to create by
hand until a token exists, and `ops/RUNBOOK.md` has it as a symptom entry.
*Rejected:* creating the records by hand and saying nothing (the next person
to read `dig` output would have to rediscover the wildcard); making
`manage_dns` a required variable (a plan-only CI run has no business
supplying a Cloudflare value).
