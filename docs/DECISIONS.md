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

**I-70. A secret or principal push to a running guest is an op, so a
lifecycle request issued in the same second can answer 409.** (merge,
2026-09-20) `PUT`/`DELETE /projects/:id/secrets/:name` and `SetPrincipals`
queue an `update_secrets` op the caller gets no id for; `stop`, `start`,
`resize` and `destroy` answer `409 conflict "an operation is in progress"`
while it runs, usually well under a second. The CLI (07) retries such a 409
for up to 10 s before surfacing it; the api test harness drains with
`WaitIdle`. *Rejected:* exempting `update_secrets` from the one-op rule
(a stop racing a secrets push is exactly what the rule prevents).
**I-71. The control plane is created now, with its Coolify pinned and its
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

**I-72. `manage_dns` defaults to false, and the absence of a record is not
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

**I-73. Points 07-cli.md and cli-config.md left implicit, settled while
building `cmd/repose`.** (07, 2026-09-20)

- *Remote normalisation lowercases the whole string, not only the host.*
  cli-config.md's rule said "lowercase host", but its own worked example
  (`git@github.com:A/B.git` and `https://github.com/a/b` both become
  `github.com/a/b`) only holds if the path is lowercased too. The doc is
  corrected to match the example, which is what a case-insensitive host
  like GitHub's actually needs.
- *Logto's OIDC endpoints live under `/oidc` relative the issuer.*
  `internal/api/auth` already hits `<issuer>/oidc/jwks` and
  `<issuer>/oidc/token` off the same `logto_issuer` value, and 07-cli.md's
  own device-code step names `POST /oidc/device/auth`; the CLI's discovery
  fetch is `<issuer>/oidc/.well-known/openid-configuration`, matching, and
  its login's OAuth client id is `repose-cli` (Native, PKCE loopback and
  device code), the same one `ops/AZURE-SETUP.md` step 12 and
  `ops/RUNBOOK.md` already name. *Not verified against a real Logto*: no
  self-hosted instance is reachable from this workstream's dev box; the
  path is inferred from the api's own two call sites, which is the closest
  evidence available. *Revisit when:* the first real `repose login` runs
  against the deployed Logto (M2's gate).
- *`repose resize SIZE` is a hidden cobra command, not documented in
  `--help`.* 07-cli.md §5.6 says it exists as "a hidden alias" and is
  "document[ed] in `features/config.md` only"; `cobra.Command.Hidden`
  is that hiding mechanism.
- *`ensureCert` recovers from `rate_limited` only when a certificate is
  already on disk with validity left*; with none, the error surfaces
  as-is (07-cli.md doesn't say what happens with no certificate at all to
  fall back to, and there is nothing sensible to reuse).
- *The Docker fixture `test/guest-sshd/` 07-cli.md §7 names is
  `internal/testguest` instead*: an in-process Go SSH server running real
  `git`, `tar` and `tmux` against a scratch `$HOME`, authenticating with a
  plain key rather than the CA certificate chain (that chain is
  `internal/ca/testca`'s and `ssh-gateway.md`'s contract, exercised by
  `cert_test.go`). The same trade 06-gateway-edge made for its own tests
  ("an in-process SSH server standing in for a guest"). *Rejected:* the
  Docker fixture (a container dependency in `go test` for behaviour a Go
  SSH server already reproduces exactly: real git, real tmux, over a real
  SSH session).
- *`PatchMeRequest.Notify.NtfyURL` is `**string`*, not `*string`: the api
  needs to tell "clear the URL" (JSON `null`) from "leave it alone" (field
  omitted), which a single pointer with `omitempty` cannot express — a nil
  outer pointer omits the field, a non-nil one pointing at a nil inner
  pointer marshals to `null`.

**I-74. The host reaches its guests through a declared `ct direction reply`
rule, not a rule an operator inserts by hand.** (01/03 follow-up,
2026-09-20) Until the gateway exists (workstream 06), an operator reaches a
guest by jumping edge → host → guest, and the host's `input` chain sends
every frame from `br-guests` to `guest_in`, which dropped all but
rate-limited ICMP: the host could open a TCP connection to a guest and
never see the reply. The M1 session worked around it with a runtime
`nft insert rule inet repose input iifname "br-guests" ct state
established,related accept` that a `systemctl reload nftables` or a reboot
removed, and that also let a repeated ICMP echo from a guest count as
established and skip the rate limit. `guest_in` now starts with
`ct direction reply ct state established,related accept`: only packets in
the reply direction of a flow the host itself opened match, so a guest's
own first packet is still dropped and the ICMP limit still holds, and the
rule survives a reload because it is in the ruleset. *Rejected:* keeping it
manual until 06 (an operator procedure that a reload silently undoes, and
`hostd` has no other way to reach a guest's sshd today); narrowing it to
tcp sport 22 (the host also curls a guest's noVNC relay, and "replies to
what the host opened" is the honest rule).

**I-75. `repose.host.apiCAFile` names an api CA that only exists at run
time.** (01/03 follow-up, 2026-09-20) `repose.host.apiCA` puts a PEM in the
store, which is how host-01 names the `hostdev` CA (I-40), but the CA the
host-services VM test registers against is generated when its `hostdev`
state is built, and reading it back at evaluation time would be an import
from derivation in every `nix flake check`. The option takes a path
instead, passed straight to `hostd --api-ca`, and an assertion refuses both
being set. It is also what a host whose CA is delivered beside the join
token needs. *Rejected:* IFD on the generated CA (a `flake check` that
builds a derivation to evaluate); a fixed CA keypair committed under
`nix/hosts/tests/fixtures` (a private key in the repository, and `hostdev`
has no flag to adopt one).

**I-76. hostd takes an identity `repose-register.service` wrote while it was
running.** (01/03 follow-up, 2026-09-20) `EnsureIdentity` read the state
directory once and then looped on the join token alone, so a hostd that
started before the token arrived kept logging `waiting for join token` for
ever after the register unit consumed that token and wrote `host.json`:
only a restart moved it. On a host the unit is ordered before hostd, so
this is the operator path of ops/RUNBOOK.md (write the token, then
`systemctl start repose-register`). The `ErrNoToken` branch now re-reads
the identity before waiting again. Pinned by
`TestEnsureIdentityTakesTheIdentityTheUnitWrote`, which fails on the old
code. *Rejected:* watching the state directory with inotify (30 s is soon
enough for a host that has just booted); having the unit restart hostd (a
restart in the middle of registration is what `RestartPreventExitStatus=3`
exists to avoid).
**I-77. Usage records are Stripe billing meter events, not subscription-item
usage records.** (09) `09-billing.md` §5.5 was written against Stripe's
`usage_type = metered` / `aggregate_usage = sum` prices and the
`POST /v1/subscription_items/{id}/usage_records` endpoint with `action =
increment`. Stripe has retired that model: the current API and every
maintained SDK (`stripe-go` v83, which this repo now depends on) expose
billing **meters** and `POST /v1/billing/meter_events` instead, and there is
no `usagerecord` package left to call. The shape §5.5 asked for is kept
where it matters and moved where it cannot be:

- Still three lines per period, still cents as the unit: one product, three
  metered prices at 1 cent per unit, each attached to a meter
  (`repose_compute_cents`, `repose_storage_cents`, `repose_egress_cents`),
  and one subscription per user carrying all three, created at first card
  attach with the period anchored then.
- The idempotency key §5.5 specified becomes the meter event's
  `identifier`, `usage:<project_id>:<hour>:<part>`, which Stripe enforces as
  unique over a rolling window of at least 24 hours. It is sent as the HTTP
  idempotency key as well, so a retried push is deduplicated twice.
  `usage_hours.stripe_usage_record_id` holds the `usage:<project_id>:<hour>`
  base and a row that has one is never pushed again, exactly as §5.5 says.
- The `credit_cents` price §5.5 mentioned in passing is still not created: a
  negative meter event is not allowed, so the trial credit is consumed
  before the push and the invoice carries a memo line.
- Reconciliation reads the other side back with
  `GET /v1/billing/meters/{id}/event_summaries`, which is why the
  `STRIPE_METER_ID_*` variables exist alongside the event names; without
  them `repose-admin billing reconcile` says it could not compare rather
  than reporting every account as a mismatch against zero.

*Rejected:* pinning an old `stripe-go` that still has usage records (a
payments library frozen at a version that will stop being served); writing
the retired endpoint by hand over `net/http` (the same bet, minus the
library); Stripe's newer `/v2/billing/meter_events` stream (higher
throughput, at-least-once semantics and a separate session object, for a
push of at most one event per project per hour). *Also recorded:* the
webhook endpoint must be created with the SDK's API version
(`stripe.APIVersion`, `2025-10-29.clover` today) or `webhook.ConstructEvent`
refuses every delivery as a version mismatch; `ops/AZURE-SETUP.md` step 17
says so, and the SDK's strictness is kept rather than disabled because an
object rendered under another version may deserialise wrongly, which in this
package means the wrong amount. Interfaces: `db-schema.md`, `api.md`
(`POST /billing/webhook`), `09-billing.md` §5.5.

**I-78. The rollup lives in `internal/billing`, the billing period is
anchored at signup and stored on every row, and `users.trial_credit_cents`
is a trigger-maintained projection of the ledger.** (09, amends 05) Three
places where the code the api already had did not match what
`09-billing.md` asks for, resolved in the workstream doc's favour:

- *Where it lives.* Workstream 05 built the hourly rollup as
  `internal/api/meter.Rollup` with a price table in `internal/billing`
  (I-42: "when 09 lands, the api swaps the implementation behind the same
  interfaces"). §2 puts the rollup in `internal/billing`, and that is where
  it is now; `internal/api/meter` keeps the sample ingest, which is the half
  §3 says this workstream does not own. `internal/obs/obslint` counts
  `internal/billing` as the api component, because it runs in the api
  process.
- *The period.* §5.1 says "month" is the user's Stripe billing period
  anchored at signup, not the calendar month; 05's rollup used
  `date_trunc('month')` for the cap, the egress allowance and the storage
  remainder. `users.billing_anchor` is that anchor, periods are counted from
  it with Stripe's short-month clamping (an anchor on the 31st bills on the
  28th in February and returns to the 31st in March), and `period_start` /
  `period_end` are written on every `usage_hours` row so the running-total
  query cannot drift from the rule that priced the row. The storage
  remainder resets at each period start, which is what makes a period sum to
  exactly `gb_alloc * 10` for a period of any length rather than only for a
  30-day one.
- *The balance.* §5.3 rejected caching the balance on `users` because "two
  hourly jobs racing would drift it", but `users.trial_credit_cents` exists
  and `/me` and the card gate both read it. It is kept as a projection
  maintained by an `after insert` trigger on `credit_ledger`, so it is
  updated inside the same transaction and under the same row lock as the
  ledger row; the drift the section rejected cannot happen, and
  `sum(credit_ledger.cents)` is still the balance of record. A usage debit
  that is recomputed differently on a re-run is corrected by an
  `adjustment` row, never by editing the original: the ledger is
  append-only.

Also settled here: the cap for a project that changed class mid-period is
the cap of the largest class it ran in during the period (§5.1's "bounded by
the larger class's cap"), carried as `Inputs.CapClass`; an exempt account
(I-16) accrues `usage_hours` and debits its credit ledger like anyone else
but is marked `stripe_usage_record_id = 'exempt'` instead of being pushed;
and the billing metrics keep the `repose_api_*` prefix of I-49 and I-60
rather than §9's `repose_billing_*` wording, so the two new families are
`repose_api_billing_stripe_push_backlog_seconds` and
`repose_api_billing_mismatch_cents` next to the existing
`repose_api_billing_gap_minutes_total` and `repose_api_rollup_duration_seconds`.
*Rejected:* leaving the rollup in `internal/api/meter` and adding the period
there (two packages owning one rule); computing the period from
`created_at` at read time instead of storing it (a period boundary that
moves when an anchor is corrected silently reprices history); renaming a
dozen deployed metric families for one workstream's original wording.
**I-79. The api's user-facing routes answer CORS on every response, with a
wildcard origin.** (08, found running the dashboard against a real browser)
`repose.herakraft.co` and `api.repose.herakraft.co` are different origins,
and nothing in 05's implementation or `api.md` set a CORS header, so a
browser blocks the dashboard's very first `fetch` with `blocked by CORS
policy` — a login was never enough to reach `GET /me`; every request failed
before this fix, found only because 08 ran the real Logto-plus-fetch flow
in an actual browser rather than trusting `internal/fakes/api`, which had
the same gap and returned successful test runs anyway. `Server.wrap`'s
`http` branch (not `internal`, which the gateway calls over mTLS, never a
browser) now sets `Access-Control-Allow-Origin: *`,
`Access-Control-Allow-Methods`, and `Access-Control-Allow-Headers:
Authorization, Content-Type` on every response, and answers `OPTIONS`
preflights with 204 before the request reaches the mux (which has no
`OPTIONS` handlers registered and would otherwise 404 them).
`internal/fakes/api` gets the same middleware, so a consumer testing
against the fake sees the same behavior the real api now has. *Rejected:*
an explicit origin allowlist (every route here is bearer-token
authenticated, never cookie-based, so a wildcard origin leaks no ambient
credential a page could ride on; an allowlist only adds an origin to
maintain in step with `repose.herakraft.co`'s eventual own domain, DESIGN
§16); a reverse proxy adding the header in front of Coolify (a second place
to keep in sync with the route list, for a header the api can set once).
Interface: none changed, `api.md`'s routes and bodies are the same; this is
a missing behavior the doc's "cli, dashboard, gateway" consumer list already
implied.

**I-80. `internal/fakes/api`'s catalog gains `kind` and `options`, and a
fragment containing `repose-force-eval-error` answers the first canonical
`eval_failed` message instead of applying.** (08) Two gaps between the fake
and what it fakes, found writing the dashboard's tests against it:
`CatalogItem` predated I-44 and had no `kind` or `options` field, so the
Menu tab's "Services" grouping and per-package option selects (config.md
"Menu versus fragment") had nothing to render; and `PUT /config` always
answered `applied` with no way to reach the config page's error-block and
fragment-line-highlight path (nix-build-contract.md "What the user reads")
without a real Nix evaluation. Both are additions to the fake only —
`api.md` already documented `kind`/`options`, and nix-build-contract.md's
messages are unchanged, so no interface doc moves. *Rejected:* a build flag
or admin endpoint to toggle the failure globally (a fragment-content marker
composes with parallel tests without shared state; the fake's existing
`Fail`/`FailNext` switch is per-route, not per-payload, so it cannot express
"this specific fragment fails").
**I-81. The gateway's metrics live in `internal/obs/metrics` as an extended
`GatewayMetrics` family, and `auth_fail_total`'s reason enum is the union the
gateway actually distinguishes.** (06, 2026-09-20) Workstream 10 owns metric
naming (`AGENTS.md`, `workstreams/README.md`) and had already merged a
`GatewayMetrics` with five series (`repose_gateway_sessions`,
`sessions_total`, `auth_fail_total{reason}`, `dial_fail_total`,
`route_duration_seconds`) wired into `ops/dashboards/gateway.json` and the
`GatewayAuthSpike` alert. `06-gateway-edge.md` §5.5 listed a richer set under
different names (`connections_open`, `auth_total{result}`,
`dial_errors_total`, `session_seconds`, `revocation_cache_age_seconds`,
`wgsync_*`, `relay_bytes_total`). Rather than a second gateway metrics
package that the dashboards would not match, this workstream extends 10's
`GatewayMetrics` in place with the missing series (`relay_bytes_total`,
`session_seconds`, `revocation_cache_age_seconds`, `wgsync_peers`,
`wgsync_errors_total`, `hook_events_total`), keeping 10's five names for the
overlap (`connections_open` becomes `sessions`, `dial_errors_total` becomes
`dial_fail_total`, and `auth_total{result=ok}` is dropped because an accepted
relay is already counted by `sessions_total`). `AuthFailReasons` grows from
10's `{bad_cert, expired, revoked, wrong_principal, stopped, not_found}` to
the reasons the gateway can tell apart: `{no_cert, bad_ca, expired, revoked,
wrong_principal, stopped, route_error, rate_limited, not_found, bad_login,
busy}`. The `GatewayAuthSpike` alert is a `rate()` over the whole counter and
the dashboard groups by reason, so both survive the wider enum; the runbook's
`bad_cert` references become `no_cert`/`bad_ca`. The same reason string is the
`reason` field of the `auth_fail` log event. *Rejected:* a `internal/gateway`
metrics package (the dashboards reference `internal/obs/metrics`' names, and
two families for one component is what I-45 and I-59 already refused
elsewhere); mapping the gateway's finer results onto 10's six reasons (a
scan showing as `bad_cert` and an api outage showing as the same reason hides
the distinction the runbook's `GatewayAuthSpike` triage needs). Interfaces:
`internal/obs/metrics/families.go`, `06-gateway-edge.md` §5.5,
`10-observability.md` §5.

**I-82. The gateway relay closes the client channel only after the guest's
in-flight request replies are delivered, and the connection tears down guest
first.** (06, 2026-09-20) Two ordering bugs found by the §7 soak (100
concurrent relays): an OpenSSH-style client surfaces a command's exit status
only when the channel's request stream closes, so the gateway must send the
full `CHANNEL_CLOSE` (not just EOF) once the guest is done; and the reply to
a client's `exec` travels on the client channel, so a guest that finishes and
closes faster than that reply propagates would have the gateway close the
client channel first, failing the client's request with `EOF` and discarding
the buffered output. The relay therefore closes the client channel only after
(a) all guest-to-client data is flushed and (b) no client-request reply is
still in flight, and on connection teardown it closes the guest side first
and drains the relays before closing the client transport. *Rejected:* a raw
bidirectional `io.Copy` with a single close on first EOF (loses exit status
and truncates output under load); a fixed delay before closing (a race is not
a timing constant). Interface text unchanged; the behaviour is in
`internal/gateway/relay.go`.

**I-83. The control VM is a server managed by the owner's existing Coolify
instance; Coolify itself is not installed on it.** (conductor, owner,
2026-09-20) The interview settled that the control plane "will be added to
Coolify", meaning the instance the owner already runs on their personal
server (which also runs Logto, Loki and Grafana). Workstream 11's text
turned that into a second Coolify installed by cloud-init on the control
VM, and the control-plane session built and applied it. The owner caught it
on first contact. The VM now boots to what Coolify's "Validate & configure"
expects of a server: root login by key (operator keys plus the instance's
own public key, `coolify_public_key` in `prod.tfvars`), Docker Engine with
the compose plugin from Docker's apt repository, `rclone` and
`postgresql-client` for the restore procedure, the WireGuard peer, and
nothing listening but sshd. The control NSG opens 22 to
`coolify_manager_cidrs` (the instance's address, `prod.local.tfvars`) next
to `operator_cidrs`; 80 and 443 stay as they were, because the proxy
Coolify installs on the server is what terminates TLS for
`repose.herakraft.co`, `api.` and `auth.`. `terraform_data.ready` checks
those facts instead of a `coolify` container's health. Consequences: there
is no admin account, no port 8000, no `APP_KEY` and no `.env` on this VM;
all of those belong to the owner's instance and its own backup. The
platform Postgres, its R2 backup job, Logto, the api and the dashboard are
still resources of that instance deployed onto this server, so
`docs/ops/coolify.md`'s click path survives with "localhost" replaced by
the added server. The live VM was converted in place (Coolify's containers,
network, images and `/data/coolify` removed, the key added), which the
`custom_data` `ignore_changes` on the VM makes equivalent to a rebuild.
*Rejected:* a second Coolify (two control planes to upgrade, back up and
log into, for one api); Coolify's "localhost" server on the personal server
itself (the api must sit in the Azure VNet next to the edge and the hosts,
DESIGN §15). `coolify_version`, `coolify_autoupdate` and
`coolify_install_url` are gone from every root; upgrading Coolify is the
owner's existing routine, not a step here.

**I-84. Logto is the owner's existing instance at `accounts.herakraft.co`;
no Logto container, and no `auth.repose.herakraft.co`.** (owner, 2026-09-20)
The owner already runs Logto with the GitHub connector configured; a second
one would be a second user database for the same people. `LOGTO_ISSUER`
(api) and `PUBLIC_LOGTO_ENDPOINT` (dashboard) and the CLI's default issuer
are `https://accounts.herakraft.co`; the issuer claim, JWKS and token
endpoints are under `/oidc` of that. What is still created there: the API
resource `https://api.repose.herakraft.co` and the `repose-cli` (Native,
device flow) and `repose-web` (SPA) applications, plus the M2M application
the api provisions users with. The `auth.` DNS record and the Logto step in
`docs/ops/coolify.md` are gone. *Rejected:* a Logto on the control VM
(DECISIONS R4-2's text; superseded here for the identity provider only,
Postgres, api and dashboard stay on the VM).

**I-85. The api verifies `iss` as `<LOGTO_ISSUER>/oidc`.** (conductor,
2026-09-20) `LOGTO_ISSUER` has always been the Logto *endpoint*: the api
fetched `<it>/oidc/jwks` and posted to `<it>/oidc/token`, and the CLI
discovers `<it>/oidc/.well-known/openid-configuration`. But the verifier
compared the token's `iss` with the bare endpoint, which only
`internal/fakes/logto` ever minted; real Logto (and `test/fake-logto`, the
dashboard's fake, which copies it) sets `iss = <endpoint>/oidc`. Every real
token would have been refused as "invalid issuer" at M2. The verifier now
expects `<endpoint>/oidc` and the api's fake mints that. The variable keeps
its name; renaming it would touch every env example and the docs of three
workstreams for no behaviour change.

**I-86. The owner's Coolify reaches the control VM over Tailscale; the
public-IP rule is the fallback.** (owner, 2026-09-20) The NSG rule for
`coolify_manager_cidrs` needs the owner's personal server to keep a fixed
public address, which it will not when that server moves, and the failure
mode is a silent hang in "Validate & configure". The owner's server and dev
box are already on a tailnet. cloud-init installs Tailscale on the control
VM (not joined: the auth key is a secret and `custom_data` is readable
through IMDS); the owner runs `tailscale up` once over SSH and adds the
server in Coolify by its tailnet address. Nothing about the owner's server
appears in any file, git-ignored or not, and port 22 on the public IP stays
operators-only. `coolify_manager_cidrs` remains for a Coolify that is not on
the tailnet. *Rejected:* a Tailscale auth key in cloud-init (secret in the
VM model); Tailscale SSH replacing sshd for Coolify (Coolify wants a plain
key it holds).

**I-87. Postgres and its backup are one Docker Compose resource from
`ops/coolify/postgres/docker-compose.yml`; api, api-grpc and web stay
separate Coolify Dockerfile applications with rolling deploys; the grpc api
issues its own server certificate; the Logto M2M application is
`repose-api`.** (owner, conductor, 2026-09-20) The owner wants the
deployment in files, not built by hand in Coolify's UI, and each thing
deployed on its own. A first draft put everything in one compose file; the
owner rejected it within the hour: a compose deploy recreates services
instead of rolling them, and one file means one deploy for all. So: the
database, which has no rolling deploy to lose, is a compose file in the
repository added as a Coolify Service (Postgres alone, the *service* named
`repose-postgres` and "Connect to predefined network" on: Coolify's parser
overwrites `container_name` with `<service>-<uuid>` and drops network
`aliases`, but Compose aliases every service by its name on each network
it joins, so the service name is the hostname that survives on the shared
`coolify` network where the applications already are); its backup is Coolify's
own scheduled dump to R2 on that service, which the owner already runs
elsewhere and which `repose-backup-check` verifies, rather than two
sidecar services the first draft wrote and the owner struck the same day
as code to maintain for a thing Coolify does; the three applications are Coolify "Dockerfile" builds
from the repository, each with an env file in `ops/coolify/` to paste,
reaching Postgres by that name with the password as a project shared
variable. Coolify's own health check cannot run in the distroless api
image (no curl or wget), so the image carries `HEALTHCHECK CMD api
-healthcheck`, a probe of its own listener, and Coolify's check is turned
off on `api` and `api-grpc` so the image's drives the rolling deploy.
Found on the way: `Validate()` refused the grpc app without certificate
files that nothing in the repository could issue (`repose-admin ca` signs
host and client certificates only), while `hostmgr.TLSConfig` already
issues a server certificate from the CA in Postgres when no file is given;
the requirement is now `GRPC_SERVER_NAMES`, and hosts verify that name
(`apiServerName`). The M2M application the api provisions users with is
`repose-api`. *Rejected:* one compose resource for everything (above);
Coolify's API driven by a script (unstable across 4.x, untestable from
here); pg_dump and rclone sidecars in the compose file (code to maintain
for what Coolify's backup does; the file is a Service rather than a git
application precisely so that Backups tab exists for it).

**I-88. The api reaches Postgres as `repose-postgres-<service uuid>`, the
container name, not the service name.** (conductor, 2026-09-20) I-87
assumed Compose's service-name alias would exist on the shared `coolify`
network once "Connect to predefined network" was on. It does not: Coolify's
generated compose lists only the resource's own network, and the shared
one is joined afterwards by `docker network connect`, which registers the
container name alone. Verified on the control VM (busybox on `coolify`:
service name NXDOMAIN, container name reachable on 5432); the first api
deploy failed on exactly this lookup. The env files carry the container
name, with the uuid pasted from the resource's Coolify URL. *Rejected:*
aliases in the compose file (stripped by the parser, and the shared network
is not in the file anyway); a `docker network connect --alias` by hand
(lost on the next redeploy); putting the applications on the resource's
network (Coolify applications have no such setting).

**I-89. The Postgres compose file joins the `coolify` network itself; the
api reaches it as `repose-postgres`. Supersedes I-88.** (owner, conductor,
2026-09-20) I-88 read the parser wrong: its "ignore aliases" is about
top-level network definitions, and serviceParser passes a service's own
`networks:` map through, only appending the per-resource network. With
`networks: { coolify: { aliases: [repose-postgres] } }` on the service and
`coolify` declared external, Docker registers the service name on the
shared network at `compose up`. Verified on the control VM with a
throwaway compose beside the live database: aliases on `coolify` were the
container name, `repose-postgres` and the explicit alias; a busybox
reached 5432 by name. The owner's preference, and the right one: the
hostname is a name chosen in a file, not a uuid copied out of a URL into
three env files. "Connect to predefined network" stays off for the
Service; its after-the-fact `docker network connect` is what registered
the container name alone. *Rejected:* the container-name host of I-88
(changes when the Service is recreated, and lives in every env file).

**I-90. The api bootstraps itself: it applies pending migrations at start
and generates the platform CA when none exists, both idempotent and
serialised across replicas on advisory locks.** (owner, conductor,
2026-09-20) 05 §5 had migrations as a Coolify pre-deploy command and
`docs/ops/coolify.md` had `repose-admin ca init` as a step after the first
deploy. Coolify's source (`ApplicationDeploymentJob::run_pre_deployment_command`,
4.3.23) runs that command with `docker exec` in a currently running
container of the app and skips it when there is none: on the first deploy
nothing ran, and the api exited on "relation secrets does not exist"; had it
survived, it would have exited on the missing CA, and an unhealthy
container is removed before anyone can exec into it. Both documents assumed
Coolify behaviour nobody had read. Now: `API_MIGRATE` (default `1`) makes
the api call `db.MigrateUp` at start, and a missing CA triggers `ca.Init`
under `LockCAInit`; a replica that loses the lock waits and loads what the
winner wrote. Writing the test for two replicas against an empty database
exposed two pre-existing races that the same fix closes: `MigrateUp` chose
the pending set before taking its lock (now re-checked under it, "already
applied" is a skip), and `EnsurePartitions` ran `create table if not
exists` outside any lock (now one transaction under `LockPartitions`).
`repose-admin db migrate` and `ca init` remain for operators; `ca init`
now refuses with a typed `ErrAlreadyInitialised`. The owner's condition
was idempotence; `internal/api/app/bootstrap_test.go` starts two replicas
at once and a third afterwards and asserts one schema and one CA.
*Rejected:* a Coolify one-off command (needs a running container); an init
container in a compose file (the api applications are single-container
Dockerfile apps by I-87).

**I-91. The api's Key Vault policy is Get, WrapKey and UnwrapKey.**
(conductor, 2026-09-20) The first CA init in production failed with 403
"does not have keys get permission": the api reads the key's current
version with GetKey before every wrap, and the policy, written as
"wrap/unwrap only, never get", withheld it. That rule was borrowed from
secrets, where Get returns the value; for a Key Vault key, Get returns the
public half and attributes, and the private key is non-exportable whatever
the permission, so Get costs nothing. Added rather than reworking the wrap
path to infer the version from WrapKey's response, which would leave the
rewrap job (which needs the current version without wrapping anything)
with the same need. The policy was also never applied: `api_identity_object_id`
was in `prod.local.tfvars` after the last apply. Applied 2026-09-20.

**I-92. Hosts dial the api at the control plane's VNet address and get
WireGuard from registration; the edge reaches `/internal` over a static
tunnel peer that `wgsync` keeps; the production edge's facts live in
`nix/edge/edge-01.nix`.** (m2 integration, 2026-09-20) The docs left the
host bootstrap circular: a host reaches the api "over WireGuard"
(`ops/coolify/README.md` puts gRPC on `10.255.255.1`), but its own WireGuard
key, address and the hub's peer arrive in `RegisterResponse`, and the edge
only admits a peer `wgsync` has read from `/internal/hosts`, which lists a
host after it registered. Settled as follows:

- *Hosts register and stream over the VNet.* `repose.host.apiAddr` is the
  control VM's private address on 8443 (`control_private_ip`, allocated
  statically as the subnet's first usable address so it can be written into
  a host's configuration), `apiServerName` is `api.repose.herakraft.co` and
  `apiCA` is the platform x509 host CA certificate (public, printed by
  `repose-admin ca show`). Azure's `AllowVnetInBound` admits it; the control
  NSG never opens 8443 on the public IP. Registration then returns the
  WireGuard material, `repose-host-net` brings `wg0` up, and the edge adds
  the peer within 30 s. WireGuard carries the gateway-to-guest path and
  observability, never the api. *Rejected:* the edge's key and endpoint in
  the host's Nix config so `wg0` is up first (the host's own key and address
  are assigned by `Register`, and the edge accepts no peer it has not been
  told about, so nothing could travel before registration either way); a
  bootstrap hop through the edge on the VNet (a second path for one RPC
  when the api is on the same VNet); the public name of DESIGN §7 (8443 is
  neither behind Coolify's proxy nor in the NSG). *Revisit when:* a host
  outside the VNet (Hetzner, R3-20) needs 8443 reachable from its address.
- *`GRPC_SERVER_NAMES` carries the addresses as IP SANs*
  (`api.repose.herakraft.co,10.255.255.1,10.200.3.4`; `pki.IssueServer`
  already made an IP an IP SAN), so the gateway, which dials the WireGuard
  address by IP, and hostd both verify the certificate the `api-grpc` app
  issues at start without a server-name override.
- *The edge's `API_URL` is `https://10.255.255.1:8444`* (option
  `repose.edge.controlWgAddress`), not the public name: Coolify's proxy
  terminates TLS for that and cannot present the gateway's client
  certificate (I-42). The control VM ⇄ edge tunnel has a static edge-side
  peer (`repose.edge.staticPeers`, declared to `networking.wireguard` and
  passed to `wgsync` as `WG_STATIC_PEERS`, which it never removes: without
  that the reconciler tore down the tunnel it reads `/internal/hosts`
  through, every 30 s). The VM side is written by hand on a live VM
  because its `custom_data` is in `ignore_changes`, so
  `edge_wireguard_public_key` only reaches a VM created after it is set;
  `infra/README.md` "Wiring the control plane to the edge" has the file.
- *`nix/edge/edge-01.nix`* holds what makes the production edge differ from
  the module (operator source addresses, the control plane's public key)
  and is imported by `nixosConfigurations.edge`: the attribute keeps its
  name because `infra/azure/modules/edge` re-runs nixos-anywhere when
  `flake_attr` changes, and a live edge must never be reinstalled by a
  rename. The operator `/32` is the first committed copy of a value
  `prod.local.tfvars` keeps local (`operator_cidrs`); the NSG stays the
  outer gate with the same list. *Rejected:* a runtime file under
  `/var/lib/repose/edge` read into an nftables set by a unit (machinery
  for one address, and one more file a reinstall would lose).
- *`repose-admin ca show` and `ca sign-server`* exist because the edge needs
  the CA certificate (`api-ca.pem`, and every host's `apiCA`) and a server
  certificate for the hook-ingest listener, and nothing printed either.
- *`bootstrap.enable` stays on for host-01*: the token delivery and the
  post-install checks reach a host on the VNet through the edge, and a
  reinstalled or re-tokened host needs that path again; sshd binds the
  WireGuard address alone once `host.json` carries it (`network.nix`),
  so the provider-NIC rule admits nothing after registration.

**I-93. hostd hands the base checkout to the build user.** (m2
integration, 2026-09-20) The first `Build` on host-01 (a one-line
home-manager fragment against main, through `hostdev build`) failed before
evaluation: hostd clones the base as root, the evaluation runs as
`nixbuild` (I-45), and Nix's libgit2 refuses a `git+file://` repository
owned by another user (`repository path ... is not owned by current user
(libgit2 error code = 7)`). No unit test could see it: the fake runner's
clone has no owner. `ensureBase` now runs `chown -R <user>: <checkout>`
after a clone and on an existing checkout too, so a checkout an operator
placed by hand (`ops/RUNBOOK.md` "Build: base unavailable") is handed over
the same way. With that, the build ran through: eval plus build of the
6.0 GB guest closure, on a store that already held the M1 base, took 35 s (eval 13.0 s, build 21.9 s), recorded in
`docs/RESEARCH.md` §11. *Rejected:* `safe.directory = *` in a git config
for the build user (libgit2 honours it, but a directive that disables the
check everywhere for a user that evaluates tenant input is the wrong
direction); cloning as the build user (hostd would need the deploy key
readable by that user, which is the key a tenant's evaluation runs next
to).

**I-97. `/run/repose` is 0755; the join-token delivery no longer makes it
0700.** (m2 integration, 2026-09-20) The first `repose-admin hosts smoke`
against host-01 on the real api built its guest in 19 s and then failed
`CreateGuest` at step 8: virtiofsd logged `/run/repose/store-export does
not exist`. The export was there; `/run/repose` had just been recreated
`0700 root` by infra's token-delivery provisioner (`install -d -m 0700`),
so the unprivileged `virtiofsd` user (I-48) could not traverse to it, and
virtiofsd reports a failed `stat` as "does not exist". It never showed on
M1 because that host's `/run/repose` had been made by `repose-host-net`
(0755) before the token arrived, and the M1 guests were created hours
later; on M2 the re-tokening came after the reboot-free switch and the
first create followed within a minute. The directory holds the export, the
rendered network files and the control socket, none of them secret (the
token file inside stays 0600); the provisioner, the runbook's by-hand line
and `repose-host-net` (which now `chmod 0755`s the directory whatever made
it) agree on 0755. *Rejected:* moving the token to its own 0700 directory
(a second path in the runbook, the host module and hostd for one file's
mode, which the file already carries).
**I-94. A scrape is a forwarded packet, so the edge needs a forward rule;
the control plane is `10.255.255.1` on the hub, and its two applications
are two scrape targets.** (m3-web, 2026-09-20) `ops/prometheus/
wireguard-peer.conf` said in as many words that "the edge's firewall needs
nothing new" for the monitoring peer. Read against the edge as built, and
against the live edge's ruleset, that is wrong three times over, and each
of the three would have presented as the same symptom: a WireGuard
handshake that looks perfect and a Prometheus with every target down.

- *The forward chain.* `nix/edge/default.nix` gives `forward` a policy of
  drop with `ct state established,related accept` and nothing else, and
  its comment says why: "hosts never route through the edge to one
  another". But Prometheus is not in Azure and hosts have no inbound, so
  every scrape of a host, and of the control plane, is a packet the edge
  forwards from one peer to another, and so is every Fluent Bit push to
  Loki. Both were dropped. `repose.edge.monitoring.{peerCIDRs,
  scrapePorts, logPorts}` adds exactly two rules when a monitoring peer is
  declared and none when it is not: that peer may reach 9100, 9101, 2021,
  9103 and 9104 on another peer, and another peer may reach 3100 on it.
  *Rejected:* `iifname "wg0" oifname "wg0" accept` (it would also let one
  tenant's host reach another's, and the control plane's gRPC listener,
  which is the thing per-host AllowedIPs and this policy exist to
  prevent); a route on the monitoring server straight to each host (hosts
  have no address anyone outside the mesh can route to, which is the
  point).
- *The control plane's address.* The same file, and the `api` job of
  `ops/prometheus/prometheus.yml`, put the control VM at `10.255.0.2`.
  DECISIONS I-92 put it at `10.255.255.1` and that is what `wg show` on
  both machines says today. The file with the wrong address was the one an
  operator would have followed.
- *One job, two targets.* `api` and `api-grpc` are separate Coolify
  applications from the same image (I-2), so they are two processes with
  two registries, and the families split between them: the HTTP families
  come from `api`, the stream, ops and outbox families from `api-grpc`.
  The job now has both with an `app` label. Verified on the control VM:
  `api` serves 57 `repose_*` series on its container address and
  `api-grpc` 62 on the host's 9104.

Also found and *not* fixed here, because it is a Coolify field and this
session does not touch the UI: `ops/coolify/README.md` prescribes a
`9103:9103` port mapping on the `api` application and the live application
has no port mappings at all, so its metrics are reachable only from inside
the container network. `api-grpc`'s `9104:9103` is in place. One field,
for the conductor.

Interfaces: none. `ops/prometheus/prometheus.yml`,
`ops/prometheus/wireguard-peer.conf` and `nix/edge/default.nix` change
together because they are three halves of one path.

**I-95. `RegisterResponse` carries `loki_url`, from a setting an operator
records with `repose-admin edge loki`; Fluent Bit refuses to start without
one.** (m3-web, 10 and 05, 2026-09-20) `docs/interfaces/host-conventions.md`
has documented a `loki_url` field of `host.json` since workstream 01,
`nix/hosts/network.nix` renders `LOKI_HOST` and `LOKI_PORT` from it,
`nix/hosts/fluent-bit.nix` uses them as its Loki output's address, and
`ops/RUNBOOK.md`'s FluentBitStuck entry says in as many words that the value
"comes from `loki_url` in `host.json`, which the api sends at registration".
Nothing sent it: `RegisterResponse` had six fields and none of them was
this one, and `internal/hostd/register` declared the struct field and never
assigned it. Every host would have rendered `LOKI_HOST=` and shipped
nothing, and the symptom — a Fluent Bit retrying a connection to an empty
host name for ever — is indistinguishable in the journal from a Loki that
is down. So M3's "Fluent Bit on host-01 ships journald and guest console
logs" could not have been closed by configuration alone.

- *A setting, not an environment variable.* The Loki names a machine
  outside this deployment, it changes without the api changing, and
  moving a log sink should not need a redeploy of the api — the same
  three reasons the edge's WireGuard endpoint and public key are already
  settings written by `repose-admin edge init`. `repose-admin edge loki
  [URL]` prints, records, or (with an empty string) clears it, refuses a
  URL with no scheme because that is the mistake that produces a fleet
  shipping nowhere, and writes an `audit_log` row like every other admin
  action. *Rejected:* a `LOKI_URL` variable in `ops/coolify/api.env`
  (a redeploy of the api to change where hosts send logs, and the api
  redeploy is the one this milestone coordinates most carefully); a
  per-host column (there is one Loki, and a per-host value is a per-host
  mistake).
- *`Rotate` carries it too.* A host registers once, so a Loki recorded
  after the fleet exists would never reach it. `Rotate` runs every 30
  days and already returns a `RegisterResponse`; it now carries the
  current value, which bounds "an operator recorded a Loki" to at most a
  month, and the runbook's edit-and-restart is the immediate path.
- *Empty is still the old behaviour, both ways.* An api that predates the
  field sends nothing, and hostd then keeps whatever `host.json` already
  had rather than clearing a working host's sink at its next rotation; a
  host that has never been told renders an empty `LOKI_HOST`, and
  `fluent-bit.service` now refuses to start with that reason in the
  journal instead of retrying nothing for ever. *Rejected:* defaulting to
  the edge's address (the edge is not a log store, and guessing an
  address is how a fleet ships to a machine nobody is reading).

Interfaces: `grpc-hostd.md` (`RegisterResponse.loki_url`),
`host-conventions.md` (where the field comes from and what an empty one
means), `proto/repose/hostd/v1/hostd.proto`. The old shape stays accepted:
field 7 is additive and an absent value means what it meant before.

**I-96. Where `features/` promised a dashboard that was never specified,
the feature doc is corrected, not the dashboard.** (m3-web, 08, 2026-09-20)
The M3 web session's last item is "`docs/features/*` match what is live".
Two of them did not, and in both cases the feature doc was written before
`workstreams/08-dashboard.md` §5.2 fixed the page list, and promised more
than §5.2 ever asked for:

- `features/status-and-logs.md` "Dashboard" promised a sortable project
  list with a cost sparkline, and a project page with a state timeline,
  a ports card and a cost breakdown by meter, and an account page
  carrying limits. §5.2 specifies none of those: the list is a plain
  table, the project page is seven cards, secrets and config are their
  own pages, and billing, settings and account are three pages. The
  section now describes the built pages route by route, and each promise
  it dropped is named in "Deferred" rather than deleted, so the next
  person to want a state timeline finds that it was considered.
- `features/notifications.md` promised the project page would show
  "delivery status per channel, so a user who got nothing can see ...
  whether the delivery failed". `GET /projects/:id/events` returns
  `{id, ts, kind, agent, summary}` and has no per-channel outcome in it;
  the outbox's state is in `events_outbox`, which no user route exposes.
  The doc now says what the Events card does answer (did the event
  happen), points the other half at `ops/RUNBOOK.md` "No notifications
  arriving", and defers the feature with the route change it needs.

*Why the doc and not the code:* AGENTS.md's rule is that the code follows
the docs, and the tie-break when two docs disagree is the one that owns
the thing. `workstreams/08-dashboard.md` owns the dashboard, its §9
checklist is what 08 was built and tested against, and `features/` is
meant to describe user-visible behaviour rather than to widen scope by
prose. Building four features in an integration session to make a sketch
true is the wrong direction, and shipping a doc that describes a product
nobody has is worse than shipping a shorter doc. *Rejected:* building
them (scope, in a session whose job is to close what exists); deleting
the promises without trace (the reasoning behind per-channel delivery
status — a support call the platform cannot otherwise answer — is worth
keeping).

**I-100. A first sign-in without a GitHub identity gets a `user-<sub>` handle;
`repose-admin users rename` and `projects destroy` exist for the operator
to put that right.** (m2 integration, 2026-09-20) Before the gate, the
production database held one user, `user-c7fh26yzrl93`, from the owner's
afternoon dashboard sign-in. Logto's Management API for that subject shows
`identities: []` and `username: null`: the account was created with email,
not through the GitHub connector, so `auth.Provisioner`'s fallback did what
it says, and the handle, which is the SSH login suffix and the certificate
`key_id`, is fixed at first sign-in and never rederived. A user row cannot
be deleted (the trial credit's ledger row references it and the ledger is
append-only, I-78), so the repair is `repose-admin users rename OLD NEW
[--github-login L]`, refused once the user has projects. The same session
found no admin way to remove a project whose create failed (`projects` had
start/stop/restart/snapshot/resize/move/restore/exec); `projects destroy
ID` enqueues the op `DELETE /projects/:id` would. Both are listed in
`repose-admin`'s usage. What the owner does in Logto: sign in with GitHub
(Console → Sign-in experience → Sign-up and sign-in: GitHub under social
sign-in, and the GitHub connector enabled), or link GitHub on the existing
account; either way the api's row keeps its handle until renamed.
*Rejected:* rederiving the handle at every sign-in (a login name that
changes under a user's SSH config); deleting the user (the ledger);
provisioning only when a GitHub identity is present (an email sign-in to
the dashboard must still work, and the handle fallback is the documented
shape for it).
**I-98. CLI releases are GitHub releases of the `Heracraft/factory`
repository, cut from `v*` tags; the dashboard serves `install.sh`.**
(conductor, 2026-09-20) The M2 gate's first step was to install the CLI,
and `install.sh` pointed at `heracraft/repose`, a repository that does not
exist, with no release to point at and no route serving the script at the
URL the landing page prints. Three fixes: `install.sh` names the repository
as it is (the rename to `repose` stays the owner's step; GitHub redirects
the old name afterwards, so the script keeps working through it);
`.github/workflows/release.yml` runs GoReleaser on a tag push and publishes
the four archives plus `checksums.txt`; the dashboard's build copies
`install.sh` into its static assets, so `curl -fsSL
https://repose.herakraft.co/install.sh | sh` is what the page says it is.
`v0.1.0` is the first tag. *Rejected:* waiting for the rename (the gate is
today); serving the script from the api (the page prints the dashboard's
host, and a static file needs no code).

**I-99. The CLI's OAuth client id is Logto's App ID for `repose-cli`, a
config value with that default, recorded in the credentials file.**
(conductor, owner, 2026-09-20) The first `repose login` of the M2 gate
answered `oidc.invalid_client: invalid client repose-cli`: the CLI sent the
application's *name* as `client_id`, and Logto identifies applications by
an opaque App ID it assigns (`jccig5bb3i4d78bq4farv` for `repose-cli`, the
way the dashboard bakes in `PUBLIC_LOGTO_APP_ID`). The id is public, so it
is the built-in default; `logto_client_id` in `config.toml` overrides it
for another Logto; and `credentials.json` records the id its refresh token
was issued to, so a refresh needs no config and an old file (no field)
falls back to the default. Interface: `docs/interfaces/cli-config.md`.
Shipped as v0.1.1. *Rejected:* a Logto application whose id equals its
name (Logto does not offer that); reading the id from the api at login (a
second round trip before the first, for a value that never changes).

**I-101. `repose login` uses the device-code flow by default; the loopback
PKCE flow is `--browser`.** (conductor, owner, 2026-09-20) The second
attempt at the M2 gate answered `oidc.invalid_redirect_uri`: the
`repose-cli` application in the owner's Logto is a Native app with device
flow enabled and, as that app type's settings page shows, no Redirect URIs
field, so the loopback redirect the PKCE flow registers on the fly can never
match. Logto also matches redirect URIs exactly, so the doc's
`http://127.0.0.1:*/callback` was never registrable. Device code needs
nothing registered, works in every terminal (including over SSH) and is what
`gh auth login` does; it is now the default, and `--browser` remains for a
Logto application that does register loopback URIs (07 §5.2 step 2 is
demoted to that case). *Rejected:* a fixed loopback port registered in Logto
(collides with anything else on the laptop, and a second Logto still needs
the entry); detecting the failure and falling back (Logto renders the error
in the browser and never redirects, so the CLI would wait on a callback that
never comes).

**I-102. Every Logto token request from the CLI carries
`resource=https://api.repose.herakraft.co`.** (conductor, owner, 2026-09-20)
The third attempt at the M2 gate logged in through the device flow and then
failed with `unauthenticated: invalid token` from the api: the CLI sent the
`resource` parameter only on the browser flow's authorization request, so
the device-code request, its token poll and the refresh grant got an opaque
token for Logto's userinfo endpoint, not a JWT with the api as audience,
and the api's verifier (I-85) refused it. `resource` now goes on the device
authorization request, the device-code and authorization-code token
requests and the refresh grant; `internal/fakes/logto` and
`test/fake-logto` both key the audience on it, so the CLI's tests assert the
shape. Shipped as v0.1.3. *Rejected:* accepting opaque tokens at the api by
calling Logto's userinfo (a round trip per request and a token that any
Logto application could mint).
**I-103. Coolify owns the backups, the destination is the owner's own S3
storage, and no credential for it comes through this repository or an
agent session.** (owner, 2026-09-20) `infra/r2` created a bucket, and
`AZURE-SETUP.md` step 10, `ops/coolify.md`, `ops/coolify/README.md`, the
control VM's `repose-backup-check` and the m3-web brief all assumed an R2
API token would arrive and be used here. The owner's decision is that it
will not: the backup destination is an S3 storage configured in their own
Coolify — quite possibly one that already exists for their other
databases — and nothing on the repose side ever holds a credential for
it. This is the same rule as I-21 and the "secrets have three homes"
rule, applied to a credential that had been treated as merely
inconvenient rather than as out of scope.

What changes:

- *`repose-backup-check` checks the near end, with no credential.* It
  used to list an R2 bucket through an rclone remote an operator had to
  create from the token. It now reports the age of the newest dump
  Coolify has written under `/data/coolify/backups` on the control VM,
  and says in its own output that this proves the dump was **taken**,
  not that it was **uploaded**. That is worth keeping rather than
  deleting: a dump that was never taken is the failure that silently
  leaves no restore point at all, and it is invisible in Coolify's UI
  until someone looks. The upload is the Backups tab, which is also
  where Coolify reports its own failures, and `RUNBOOK.md`
  "PostgresBackupStale" now walks both halves and says which tool
  answers which. *Rejected:* deleting the check (it would have left the
  near-end failure unwatched); having it call Coolify's API (unstable
  across 4.x, and it would need a Coolify token on the VM, which is the
  same problem one layer along).
- *`ops/restore-rehearsal.sh` takes a file.* The dump comes from the
  database's Backups tab, which is where a human already is when they
  need a restore. `--from-bucket` stays for whoever has a remote of
  their own, because it is three lines and removing it would not make
  anything safer.
- *`infra/r2` stays, marked optional and unused by production*, rather
  than being deleted: a second environment or a different owner may want
  a bucket, the module is written and validated, and deleting a working
  module to express a policy is how the policy gets re-litigated by
  someone who needs the module. Its header says plainly that production
  does not use it. `backup_bucket` is gone from
  `infra/azure/modules/{environment,coolify}`, since nothing on the VM
  reads a bucket name any more; `backup_max_age_hours` stays.

*Why this is better than the arrangement it replaces:* the credential
that would have been pasted into an agent's tfvars, an operator's shell
history and an rclone config on a VM now exists in exactly one place the
owner already manages. The cost is that this side cannot prove the
upload happened — which is honest, because it never really could: an
rclone listing proves an object exists, not that it is last night's
database.

**I-106. `repose run` waits on the op a create leaves in flight instead of
starting the project.** (conductor, 2026-09-20) The first real `repose run`
answered `conflict: recruiting is already starting`: the api's create
returns `state: creating` with the create op's `op_id`, and the CLI's
`ensureRunning` saw "not running" and issued a start, which the engine
refuses while the create op runs. The fakes never showed it because their
create completed instantly. The CLI's `Project` now carries `op_id`;
`ensureRunning` waits on that op (streaming its build log) or, without an
op id, on the state leaving `creating`/`starting`, then re-reads before
deciding to start. `internal/fakes/api` gains `Options.CreateDelay`, which
answers a create the way the engine does and refuses a start meanwhile; the
regression test runs the flow against it. *Rejected:* retrying the start
until it is accepted (the op-conflict retry already exists and would have
raced the create's own start for the whole build).

**I-107. guestd's `SetupProject` points `origin` at the project's remote.**
(conductor, 2026-09-20) The first real sync failed in the guest with
`'origin' does not appear to be a git repository`: guestd ran `git init` in
the project directory and never added a remote, while the CLI's sync (and
`docs/features/sync-at-launch.md`) expect "a clone with an origin remote".
`Setup` now adds `origin` as the SSH form of the api's normalised remote
(`github.com/owner/repo` becomes `git@github.com:owner/repo.git`, fetched
through the agent the CLI forwards), or `set-url`s an existing one.
Guests built before this carry the old guestd until the next base version;
the M2 test guest had its remote added by hand. *Rejected:* cloning at
setup (the CLI's first sync fetches exactly what it needs and the agent is
only present during a run); HTTPS URLs (private repositories need the
user's credentials, which only the forwarded agent carries).

**I-108. The generated `~/.ssh/repose/config` sets `IdentitiesOnly yes`.**
(conductor, 2026-09-20) `ssh <slug>.repose` from a plain terminal answered
`permission denied (certificate required)` while the CLI's own connection
worked: with an ssh-agent loaded, ssh offered the agent's plain key before
the configured certificate identity, and the gateway refuses plain keys by
design. The generated host block now restricts ssh to the certificate
identity it names; `ForwardAgent yes` stays, since the agent is still
forwarded for the guest's own git.
**I-104. The CLI sends an IANA zone name or no `tz` at all.** (conductor,
owner, 2026-09-20) The owner's first `repose run` failed at create with `tz
is not an IANA zone name`: Go names `time.Local` "Local" unless `TZ` is
set, and the CLI's fallback sent the zone abbreviation ("EAT"), which the
api rightly refuses. `localTZ` now resolves `TZ`, the Local name, the
`/etc/localtime` symlink or `/etc/timezone`, validates each with
`time.LoadLocation`, and omits `tz` when none is known so the api applies
its default. *Rejected:* accepting abbreviations at the api (ambiguous:
"CST" is three zones).

**I-105. The provisioner reads the GitHub login from Logto's
`rawData.userInfo.login`; the fake Logto emits that shape.** (m2 gate,
2026-09-20) The conductor's device-code login, approved by the owner
through "Continue with GitHub", provisioned `user-yv7ryeiczbkz` although
Logto's Management API shows a `github` identity for that subject. The
connector stores `details = {id, name, avatar, email, rawData}` with
`rawData = {userInfo, userEmails}`, and GitHub's `login` sits inside
`userInfo`; `auth.Lookup` tried `details.login` and `rawData.login`, both
absent, and fell through to the `user-<sub>` fallback of I-100. The unit
test passed because `internal/fakes/logto` rendered the identity as the
flat `details.login` no Logto version sends. `Lookup` now tries the three
shapes in order and the fake renders the real one (a `LegacyShape` flag
keeps the flat form for the fallback's own test; a `User` without a
GitHub login renders no identity, the email sign-in case). Existing rows
are not rederived: `user-c7fh26yzrl93` (an email account, no identity to
derive from) and `user-yv7ryeiczbkz` (renamed once its test project is
gone; a handle with projects is never renamed because the guest host
certificate carries `<slug>.<handle>`, I-42). *Rejected:* fetching GitHub's
profile from the api with the connector's token (the Management API
already returns it); a rename that re-signs host certificates (an
operator path for a one-time repair).

**I-110. The relay half-closes towards the client when the guest's side of
a channel ends; the fake guest reads its agent channel with one reader.**
(m2 gate, 2026-09-20) The conductor's end-to-end run found that an exec
through the gateway with agent forwarding (`ssh -A … 'ssh-add -l'`) printed
its output and never returned, which stalled the CLI's sync (`git fetch`
over the forwarded agent) for 13 minutes. `pipe` forwarded the client's
EOF to the guest (`CloseWrite` on the guest channel once the client's
stdin ended) but never the reverse: when the guest's side of a channel
ended it waited for the guest's full close and only then closed the
client's channel. For a session that works, because the program's exit
closes both directions at once. For the agent channel it deadlocks:
sshd sends `CHANNEL_EOF` when the program's agent socket closes and sends
`CHANNEL_CLOSE` only after it has received the peer's EOF; OpenSSH's client
closes its agent socket, and so sends its own EOF, only when it sees EOF
from the channel; the gateway sat between them forwarding neither. The
relay now issues `CloseWrite` towards the client as soon as the guest's
inbound data (data and extended data) has ended, symmetric with the other
direction; the full close still follows the rules of I-82. Two things about
the test: `TestRelayAgentForwarding` passed on the old relay because the
fake guest closed its agent channel outright, so the fake now ends it the
way sshd does (EOF, wait for the peer's EOF, then close), and the exec is
bounded so a regression fails in 15 s rather than hanging CI; and the fake
hands `agent.NewClient` a plain `io.ReadWriter`, because given a Closer
x/crypto v0.55 starts a pipelined reader that keeps reading the channel
after `List` returns, and x/crypto's channel EOF wakes only one of two
readers, which left the fake's own read asleep and looked, for an hour,
like the gateway bug it was masking. *Rejected:* closing the client
channel on the guest's EOF (loses the exit status ordering I-82 fixed);
a timeout on idle channels (a race is not a timing constant).
**I-109. The guest base disables NixOS's systemd ssh proxy include.**
(conductor, 2026-09-20) The first `git fetch origin` inside a real guest
failed with `Bad owner or permissions on
/nix/store/...-systemd/lib/systemd/ssh_config.d/20-systemd-ssh-proxy.conf`:
NixOS's `programs.ssh` includes systemd's drop-in from the store in every
client invocation, and over the shared virtio-fs store that file is owned
by `nobody:nogroup` as far as the guest can tell, which ssh refuses for
any config it reads. The proxy exists for reaching VMs over AF_VSOCK from
a host, which a guest never does; `programs.ssh.systemd-ssh-proxy.enable =
false` removes the include. Verified by the guest-base VM test and the
live guest on base 2026.09.20.2. *Rejected:* mapping store ownership to
root in the guest (virtio-fs's uid squashing is what keeps the shared
store read-only and tenant-safe, DESIGN §6).

**I-111. The guest pins GitHub's SSH host key and trusts other forges on
first use.** (conductor, 2026-09-20) With `origin` set by guestd (I-107) the
first real sync on the new base failed with `Host key verification failed`:
a fresh guest has no known_hosts, and git's ssh refuses an unknown host
rather than prompting inside a non-interactive fetch. The guest base pins
`github.com`'s published ed25519 key through `programs.ssh.knownHosts` and
sets `StrictHostKeyChecking accept-new` for everything else, which is the
policy a laptop's first clone applies and never overrides a pinned key.
*Rejected:* `StrictHostKeyChecking no` (accepts a changed key too);
seeding known_hosts from the laptop at sync (one more file the CLI copies,
and the laptop may never have connected either).
**I-112. Backups are entirely Coolify's, and Coolify redeploys on every
push to `main`.** (owner, 2026-09-20) Supersedes what is left of I-103,
which had kept a foot in the door: an on-VM check, a restore rehearsal
script, a runbook alert entry and `infra/r2` as an "optional" module. The
owner's position is simpler and better: the Postgres backup and its
restore are configured on the Postgres service's Backups tab in their own
Coolify, and this repository has no part in them at all.

Removed rather than reworded, because a half-owned responsibility is the
one nobody holds: `repose-backup-check` and its `backup_max_age_hours`
variable are out of the control VM's cloud-init and out of both infra
modules; the readiness provisioner no longer asserts `rclone` and
`pg_restore`, and cloud-init no longer installs them, since the restore
procedure that justified them is gone; `ops/restore-rehearsal.sh` is
deleted; `infra/r2` is deleted along with the Makefile's `ENV=r2` root and
its Cloudflare token guard (a token is still needed for `manage_dns`, a
different scope); the backup and restore sections are out of
`ops/coolify.md`, `ops/coolify/README.md`, `AZURE-SETUP.md` step 10 and
`RUNBOOK.md`, and the release checklist's item is now "configured in the
owner's Coolify; nothing here".

*What this costs, recorded so nobody rediscovers it as a surprise:* the
near-end failure — a dump that was never taken — is no longer watched
from this side, and it is invisible until someone opens the Backups tab.
I argued for keeping the check for exactly that reason and the owner
overruled it, correctly: a check that proves a dump exists but not that
it is last night's database, on a machine whose backups somebody else
owns, is a second place to look that can disagree with the first. One
owner, one place. *Also kept deliberately:* `ops/coolify.md`'s note that
Coolify encrypts its stored credentials with `APP_KEY`, so a dump
restored without it is ciphertext. That is not backup machinery, it is a
fact about the owner's own restore, and it is worth more to them now that
the whole procedure is theirs.

*And the second half.* Coolify watches the repository, so `api`,
`api-grpc` and `web` rebuild and roll on every push to `main`: a deploy
is a consequence of merging, not a step. Every "then redeploy X"
instruction is therefore wrong and is gone from `coolify.md`, the ops
README, the runbook and the launch prompts. Two survive as what they
are: a rollback (Coolify's deployment history — the one deploy nobody
gets automatically) and a re-paste of the Postgres Service, which is a
pasted resource rather than a watched one. It also means fact 13's
dropped request happens on **every merge**, not only on deliberate
deploys, which is what turns it from a curiosity into the owner's
decision about the proxy's drain. Recorded as `coolify.md` facts 15
and 16.

**I-117. Build-log flushes are serialised and `Read` always flushes first,
so a reader never misses the batch a flush is inserting.** (conductor,
2026-09-20) CI failed `TestSSELiveStreamAndConcurrentLoad` with "stream 0
saw 1 lines" of 3: `Flush` took the pending batch out under the mutex,
released it, then inserted the rows in a transaction; a `Read` in that
window saw nothing pending, skipped its own flush and queried the table
before the insert committed. With the op already finished at connect time
the SSE handler does exactly one catch-up read, so the stream ended short.
`Flush` now holds a flush mutex for its whole run and `Read` calls `Flush`
unconditionally, which makes it wait for one in flight; a test races
appends and flushes against a reader. *Rejected:* publishing to subscribers
before the insert (a subscriber would then see lines the table does not
yet have, and a `since` replay could skip them).
**I-113. `repose-admin projects create` makes a project for a synthetic,
billing-exempt user; `hosts smoke` uses the same path.** (m3 integration,
conductor, 2026-09-20) The M3 checks that must not run as the owner (two
tenants for the isolation rows, throwaway guests for notifications and
audit rows) need a guest under a user that cannot sign in, the way `hosts
smoke` runs under `repose-smoke`. `projects create --user HANDLE --name N
[--class C] [--host N] [--create-user] [--wait]` is smoke's create step
factored out: with `--create-user` a missing handle becomes an exempt
account with limits 100/100 and no Logto identity (I-16), the project and
its empty revision are inserted the way `POST /projects` does and the
create op is enqueued for api-grpc to drive, pinned to a host when asked.
Audited as `project_create`. *Rejected:* inserting rows by hand with
`psql` (the op has to come from the engine); a Logto identity for
synthetic users (a second identity path in the api for a test).

**I-114. The CLI waits through `building`, and reads an op's error as
`{code, message}`.** (m3 integration, 2026-09-20) Two things the first
`repose run` and `repose config apply` on host-01 under a real build
showed: I-106's wait covered `creating` and `starting`, but the create
op's first phase puts the project in `building` a moment after `POST
/projects` answers, so the CLI issued a start and got the api's
`conflict: m3-check is building` (the fake completed its create before
any state could be read; it now reports `building` while its create
delay runs, which reproduces the race). And `Op.Error` was a string while
the api stores and returns the hostd result's `{code, message}` (I-42),
so the secret-in-fragment refusal rendered as `error: ` with nothing
after it. `OpError` decodes both shapes, and `RenderBuildError` takes the
code and prints the prefix `nix-build-contract.md` "What the user reads"
assigns it (`config error: ` for `eval_failed` and the api's `invalid`,
`config too large: ` for `closure_too_large`, none for `build_failed` and
`build_timeout`, `error: ` otherwise). *Rejected:* changing the api to
return a string (the code is what the CLI switches on, and the dashboard
reads the same shape).

**I-115. A destroyed project's `rev-*` GC roots go with its guest, and a
restore of a destroyed project rebuilds its closure.** (m3 integration,
2026-09-20) `host-conventions.md` says the guest's root is removed on
destroy and the `rev-<project>-<revision>` roots keep the last three
built revisions; hostd removed the first and never the second, so every
destroyed project on host-01 left its revision roots (seven of them
after one evening) and 6 GB of store each, for ever. `DestroyGuest` now
prunes every `rev-<project>-*` root of that project. Because a restore
of a destroyed project (`POST .../snapshots/:sid/restore` with
`as_new_project`, within the 30-day retention) would otherwise hand
hostd a closure the next `nix-collect-garbage` may have removed, the
api copies the revision without its closure when the source is
destroyed, so the restore plan is `build, restore, start_guest`.
*Rejected:* keeping the roots for 30 days to match snapshot retention
(the store is the host's scarce resource and the closure is a
deterministic build of a fragment the database still holds).

**I-116. `repose_host_guests` publishes every state and class at zero.**
(m3 integration, m3-web, 2026-09-20) m3-web saw no `repose_host_guests`
series on host-01 at all. The wiring was fine (the gauge appeared as soon
as a guest existed: `repose_host_guests{class="small",state="running"} 1`
at 23:45Z); a Prometheus `GaugeVec` with no children exposes nothing,
not even HELP, so a host with no guests read as "no data" on the
capacity panel rather than 0. `refreshGuestGauge` now sets every
`state × class` combination to 0 before counting, so the family has
thirty series from the first refresh. *Rejected:* a separate
`repose_host_guests_total` gauge (a second name for the same count).

**I-118. `Build` carries `base_version`, hostd writes it beside the
fragment, and the flake stamps the guest with it.** (m3 integration,
2026-09-20) Every guest built by hostd on host-01 had
`/etc/repose/base-version` and its NixOS label reading `dirty`
(`repose-guest-profile` prints it, `nixos-version` shows it, the build
log names `nixos-system-repose-guest-dirty`). `nix/flake.nix` derives the
stamp from `self.shortRev`, which is present for the `git+file://`
checkout (`nix flake metadata` on host-01 as `nixbuild` shows the
revision) and absent the moment the evaluation overrides the `fragment`
input, which every hostd `Build` does (I-28); reproduced on host-01 with
hostd's exact command. The flake's comment already wanted the api's
`base_versions` label and the guest file to carry the same string, and
only the api knows that label. `Build` gains `base_version` (optional;
the old shape is accepted and keeps today's stamp), hostd writes it as
`base-version` next to `fragment.nix` after validating it as a label,
and `guestSystem` reads it into `repose.baseVersion` when present. The
api sends the revision's base version. *Rejected:* fetching the base with
`?rev=` and hoping `self.rev` survives the override (it is the override,
not the ref, that drops it); stamping the git revision from hostd (the
label users see is the version, `2026.09.20.3`, not a sha). Interfaces:
`grpc-hostd.md`, `nix-build-contract.md`, `hostd.proto`.

**I-119. The api renders the menu with `internal/menu`; `internal/nixmenu`
is gone.** (m3 integration, 2026-09-20) I-42 recorded that the api carried
its own package-only stand-in for the catalog "until 12 lands", and I-44
that "the api calls it" once it had; the swap never happened, and the
first `GET /catalog` on the real api returned the stand-in's 23 package
entries (no services, no `kind: service`, no options beyond nodejs's
version) with a generated fragment whose second line was not the
selection the feature doc and I-44 describe. `PUT /config {menu}` and
`GET /catalog` now go through `menu.Load()` (validated once per process,
`sync.OnceValues`), `Catalog.Render` and `Catalog.Public`; a `*menu.Error`
is the api's `invalid` with its message; `menu.IsGenerated` is the
menu-versus-fragment switch. A project generated by the stand-in (header
only, no selection line) still counts as menu-managed through its
`config_revisions.menu` column. The stand-in package and its tests are
deleted. Found by `ops/checks/menu.sh` on host-01, whose bun menu apply
worked either way (built in 5 s, on PATH without a reboot).

**I-120. The tap name and MAC come from a hash of the guest id, not its
first eight hex; a create refuses a tap or MAC another guest holds.** (m3
integration, 2026-09-21) Two synthetic tenants created on host-01 within
the same minute (`repose-admin projects create`, 00:00:33Z) got guest ids
`01a0c143-f202-…` and `01a0c143-f36d-…`: a UUIDv7 starts with its
millisecond timestamp, so the first eight hex, which `host-conventions.md`
made the tap name and the first four bytes the MAC, are the same for every
guest created within about 65 seconds. The second create failed at step 6
on the first guest's htb root (`tc: Change operation not supported by
specified qdisc`), and its retry attached a second Cloud Hypervisor to
`tap-01a0c143` with MAC `52:54:01:a0:c1:43`: two tenants, one tap, one MAC,
both admitted by the bridge set. A guest is a per-project microVM behind
a per-guest tap (DESIGN §7, SECURITY boundary 2); the name was the hole.
Now `tap-<first 8 hex of sha256(guest id)>` and `52:54:<first 4 bytes of
sha256(guest id)>`, and `CreateGuest` returns `already_exists` when
another record on the host has that tap or MAC rather than adopting the
device (`AddTap` was written to be re-run for the same guest and could
not tell the two apart). Existing records keep the tap and MAC stored in
`state.db`, so guests running through the hostd upgrade are untouched;
only new guests are named by the hash. *Rejected:* `tap-<index>` from the
address allocator (an index is reused after a destroy while the previous
guest's nftables objects may still be going away); the last eight hex of
the id (random, but a name should not depend on which part of an id
format is random). Interface: `host-conventions.md`. Verify on host-01
after the next host switch: two creates in the same minute, two taps,
`nft list set bridge repose guests` with two distinct tuples.

**I-121. `AgentEvent` on the host stream carries `tmux_window`, and a
Claude `Stop` without a readable transcript is summarised as "claude
finished".** (m3 integration, 2026-09-21) The first hook events from a
real guest on host-01 (`ops/checks/notifications.sh`, 00:08Z) reached the
api and were delivered to ntfy and email within a second, and every row
had `tmux_window` null and, for Claude's `Stop`, an empty summary. Two
gaps between three contracts: guestd's `AgentEvent` (vsock) carries
`tmux_window`, the `events` table and `13-notifications.md` §5.1 carry
`window`, but hostd's `AgentEvent` (gRPC, `grpc-hostd.md`) had no such
field, so hostd dropped what guestd had resolved and `repose status` could
never say which window finished. And `guest-conventions.md` says a `Stop`
summary is the transcript's last assistant line "else `claude finished`";
`mapClaude` sent the empty string instead. `hostd.proto` `AgentEvent`
gains `tmux_window = 5` (old shape accepted, empty means unknown), hostd
forwards guestd's value, the api's ingest stores it; the mapper falls back
to "claude finished". Interface: `grpc-hostd.md`.

**I-122. guestd's watcher accepts every process name an agent runs as;
Gemini CLI is `node`.** (m3 integration, 2026-09-21) On host-01 the
pane-idle heuristic (I-49) fired for pi 101 s after its window went quiet
and never for Gemini CLI: the watcher took a window for an agent's only
when the pane's process tree held a process named exactly after the agent
(`gemini`), and Gemini CLI is a bundle the guest's node runs (I-46), so
its process is `node`. The window was never an agent window, so neither
`AgentState` nor the completion existed for it. `binaries` is now a list
per agent (`gemini: gemini, node`) used by both the liveness check and the
foreground-command check. *Rejected:* matching the window name alone (a
user's shell renamed `gemini` would be reported as an idle agent, which
is what the process check exists to prevent). Verify on the next base:
`ops/checks/notifications.sh`'s gemini row.


**I-123. Gateway session reports are ordered and sent at most once.** (m2
gate, 2026-09-21) CI saw `TestSessionReportsAndCertCache` collect
`[opened, closed, opened, closed, closed]` for three connections: one
session's open never arrived and one close arrived twice. Both came from
`session.report`: the open was posted on the session's own context, so a
client that connected and left within the round trip cancelled its own
"opened" mid-flight (both attempts, since the retry shared the context),
and every report retried on any error, so a close whose answer was lost
after the api had recorded it was posted again. Nothing ordered the two,
either. Now both reports run on a context the session's end does not
cancel, the close report waits for the open report to finish, and a retry
happens only when the request never reached the api (a connection error),
never after a timeout or a refused answer. The api's `/internal/sessions`
keeps its set semantics per `(project_id, cert_serial)`, so a duplicate
would be harmless there but an open after its close would leave a
phantom `ssh_sessions` signal, which is the case the ordering closes. The
test asserts that closes never outnumber opens in report order and that no
event is delivered twice, run under the race detector twenty times.
*Recorded, not fixed:* `gateway_sessions` is keyed by `(project_id,
cert_serial)`, so several connections under one certificate count as one
session in `ssh_sessions`; a per-relay session id in the report would fix
the count and is an interface change for a later pass. *Rejected:*
retrying on every error with idempotent bodies (no session id exists to
make them idempotent).

**I-124. A destroy whose plan is empty still ends with the project
destroyed.** (m3 integration, 2026-09-21) The smoke project of the
mistyped base (create failed at the checkout, before `CreateGuest`) could
not be destroyed: `PlanDestroy` is empty for a project with no guest, the
op finished `done` at once, and the finaliser that sets `destroyed_at`
runs only from the `destroy_guest` phase, so the row stayed `error` with
its host set, and a user's `DELETE /projects/:id` on such a project would
have left them a dead project counting against their limit. `finish`
now calls the same finaliser for a `destroy` op whose project is not yet
destroyed; the phase result path is unchanged.
`TestDestroyWithoutAGuestMarksTheProjectDestroyed`.

**I-125. guestd's watcher also matches an agent by the executable's
name.** (m3 integration, 2026-09-21, amends I-122) On base 2026.09.21.1,
with `node` in gemini's name list, the gemini window still never counted
as an agent's: node renames its main thread, so `/proc/<pid>/comm` of
every Gemini CLI process reads `MainThread` (seen in m3-stamp: pids 909,
919, 1139, 1149, all `MainThread`, all `exe` = node). `treeHasComm` now
also compares the basename of the `exe` link, which names the binary
whatever the thread is called; `cmdline` and `environ` stay unread, as
`docs/SECURITY.md` promises and the strace test pins. Process samples
keep `comm` as their name. Next base.

**I-126. The api's parse-time syntax error is worded like hostd's.** (m3
integration, 2026-09-21) `PUT /config` checks syntax with
`nix-instantiate --parse` before any build (05 §5.7), so a syntax error
never reaches hostd's mapping and the user read Nix's own order, `syntax
error, unexpected ';' at fragment.nix:1:34 (fragment.nix:1)`, where
`nix-build-contract.md`, the fake api and the dashboard's tests all say
`syntax error at fragment.nix:1:34, unexpected ';'` (`ops/checks/menu.sh`
on host-01). The api's summariser now renders the contract's line and
the `invalid` message carries no `(fragment.nix:N)` suffix; the line is
in `detail.fragment_line` as before. Note for readers of the CLI's
output: it prints the local file's name in place of `fragment.nix`
(`RenderBuildError`), so the same refusal reads `at syntax.nix:1:34` on
a laptop.

**I-127. The CLI reads the op again when the build log stream ends.** (m3
integration, 2026-09-21) With I-114 in place, `repose config apply` on
host-01 still printed `error:` and nothing for a build that failed after
streaming: `waitOp` read the op once (running, no result), streamed the
SSE log, and when the stream's `done` event said `error` it returned that
stale op with its state flipped, so the code, message and fragment line
the api had written by then never reached `RenderBuildError`. A failure
the api knows at `PUT` time (the parse check, the secret-in-fragment
refusal) has no stream and was unaffected once I-114 landed. `waitOp`
now reads the op again after the stream ends.
`TestWaitOpReadsTheOpAgainAfterTheStreamEnds` (an httptest server that
answers running, streams, then answers the error).

**I-128. The CLI prints the verbatim block of a build error.** (m3
integration, 2026-09-21) `nix-build-contract.md` "What the user reads"
makes the message a summary line, a blank line, then the verbatim output,
and 07-cli.md §5.10 has the CLI print the summary, the fragment context,
then that block. `RenderBuildError` printed the summary and the context
only, so the first real closure over the cap on host-01 (`closure is
26.6 GB, limit is 20 GB; largest paths:`) named no path, and a
`build_failed` showed none of the builder's log. The block now follows
the context, newlines trimmed, indentation kept.
`TestRenderBuildErrorPrintsTheVerbatimBlock`.


**I-129. M2's two-person gate was closed with one person and a second
account.** (owner, 2026-09-21) `MILESTONES.md` asked for a second person
with a GitHub account to run `repose login` and `repose run` on their own
laptop, be refused the first person's guest, and get a notification. No
second person was available on the night, and the owner waived the clause
for M2 at 01:02Z on this evidence: the owner's own laptop run through the
released CLI (v0.1.4 by `install.sh`, device-code login on the email
account `user-c7fh26yzrl93`, `nuru-playground` and `age-calculator` created,
built and running on host-01, relays from the laptop through the switched
edge); m3's isolation suite on host-01 under two accounts (18 rows pass,
neighbour ratio 1.02, password attempts refused and logged on host and
edge); and the finished-agent deliveries to ntfy for claude, codex,
opencode and pi. What the waiver does not cover and stays open: a second
*human*'s laptop, OS keychain and SSH agent meeting the gateway, which
M5 step 3 still requires unchanged. The rehearsal that preceded the run
found and fixed twelve gate-blocking defects on the day (I-99, I-101,
I-102, I-104..I-111, I-120), which is what the two-person gate exists to
surface; the owner judged the remaining risk to be in the second laptop,
not the second account. *Rejected:* keeping M2 open until a second person
appears (M3 work on the shared host was waiting on it).

**I-130. One refused blob delete does not end the expiry run.** (m3
integration, 2026-09-21) `snapshots.Expiry.Once` looped "next expired row,
delete its blob, mark it" and returned on the first error, so the run that
hit I-131's `403 AuthorizationPermissionMismatch` (01:27:06Z, request id
`97d95da0-001e-0101-4468-49cafa000000`, the first of two aged m3-check
snapshots) deleted nothing, and would have re-selected the same row at
every run for as long as that one blob was refused: a single bad blob (a
lease, a mismatched name, a permission) would have held the whole
retention rule hostage. The run now reads its candidates once, then
handles each in its own transaction (`for update skip locked`, re-checked
against the rule so a restore that started meanwhile still holds its
snapshot); a refused delete logs `snapshot_expiry_fail` with the
snapshot id, leaves the row for the next run and moves on; the run ends
with an error naming how many rows it kept, so the job's own
`snapshot_expiry_fail` line still marks the run failed and the gauge of
oldest live snapshot still rises. `TestExpiryContinuesPastOneFailedBlob`
(fake store refusing one path). *Rejected:* marking the row deleted
anyway and letting the 45-day lifecycle rule collect the blob (the row is
the audit of what is in the container; a row that says deleted while the
blob is there is the shape 05 §5.8 forbids); retrying the same blob inside
the run (the failures seen are not transient at the run's timescale).

**I-131. The api's service principal gets Storage Blob Data Contributor on
the snapshots container.** (conductor, 2026-09-21) 05 §5.8 has the expiry
job delete expired snapshot blobs "through the Blob SDK with the api's
identity", and workstream 11 assigned that role to the hosts' managed
identity only. In production the first real expiry run (01:27Z, two
snapshots aged to 8 days on m3-check) was refused on its first delete with
`403 AuthorizationPermissionMismatch`, so the retention rule had never
deleted a blob; the lifecycle rule's 45-day delete was the only thing that
would ever have run. `modules/storage` now takes `api_identity_object_id`
(the same value `modules/keyvault` already uses for wrap and unwrap) and
assigns the role scoped to the container, created only when the id is
supplied, and `modules/environment` passes it through. Applied by the
owner with the usual `tofu apply`; until then the expiry row of 05 stays
open. *Rejected:* a custom role with only `blobs/delete` (a second role
definition to maintain for one verb, and the api already has read on the
same blobs through restore); running deletes from the hosts (the host's
identity is the one a compromised host holds, and retention is the
control plane's decision).

**I-132. A base bump that needs a reboot says so in its event.** (m3
integration, 2026-09-21) `basebump.OnOpFinished` raised `base_updated`
with the summary "base X applied" for every finished bump op, including
one whose apply ended with `reboot_required` (a kernel change on a running
guest, which 12 §5 leaves for the next stop/start): the owner would have
read "applied" on the notification while the guest still ran the old
kernel. The summary now says the base is built, that it changes the
kernel, and that it takes effect at the next `repose stop && repose start`;
the kind stays `base_updated` (13 §5 lists it, and an interface keeps its
shape one release). `TestBumpNeedingRebootSaysSo` (the fake hostd reporting
`kernel_changed`). *Rejected:* a new kind `base_update_ready` (a second
kind for one outcome of one job, which every channel and the CLI's
`events` would have to learn).

**I-134. The build phase takes its base from the revision, not the
project.** (m3 integration, 2026-09-21) `Engine.baseRef` read
`projects.base_version`, so a base bump, whose revision names the new base
while the project still records the old one, was built against the base
it was leaving, then the apply phase set the project's base to the new
version from the revision. On host-01 the 02:08Z security publish of the
kernel-flip base (38d1cbf) swept four projects in 5 s each, `build_ms`
around 330, `kernel_changed` false, no new checkout under
`/var/lib/repose/base`, and `repose-admin base status` reporting every
one "applied on 2026.09.21-m3-0208": the api believed the fleet was on a
base no guest had been built from; every earlier sweep on host-01 went
the same way, unnoticed because no published base had differed in
anything a 5 s eval would show. The revision's base wins now, then the project's,
then the newest published, then the dev checkout;
`TestBumpNeedingRebootSaysSo` asserts the fake hostd received the new
base's `nix_rev` and version in the Build. *Rejected:* setting the
project's base_version at enqueue time (a failed bump would then have
moved the project to a base it does not run).

**I-133. The api's `/metrics` is a Traefik router on the app, behind an
IP allow-list; no collector and no host port.** (owner via conductor,
2026-09-21) The `api` application publishes no port and cannot: a
published host port stops the old and the new container coexisting, so
Coolify could not roll it, which is the constraint that split `api-grpc`
off in the first place (I-2). Its 57 `repose_*` series therefore needed
a route of their own, and the choice between two was left open for the
owner. It is the router.

The proxy that already fronts the api serves `/metrics` from the same
container port, through custom labels on the app:
`Host(api.repose.herakraft.co) && Path(/metrics)` on the https entry
point, `loadbalancer.server.port=9103`, and an `ipallowlist` middleware
for `10.255.0.0/16, 10.200.0.0/16`. Nothing new runs, the rolling deploy
is untouched, and it rides the certificate Traefik already has.

*Rejected: Grafana Alloy as a Coolify service*, scraping `api:9103` and
`api-grpc:9103` by name on the docker network and remote-writing out. It
is the tidier shape on paper — the scrape never leaves the network, no
allow-list, and the same agent could later replace Fluent Bit on hosts
and the edge and make I-94's forwarding problem disappear. It loses on
two concrete grounds: it is a second resource to run, upgrade and back
up for one endpoint, and it needs a `remote_write` receiver that the
owner's Prometheus does not currently expose, so choosing it would have
meant changing the owner's stack to suit ours. The Alloy argument
survives intact for the day Fluent Bit is reconsidered; it just should
not have ridden in on this decision.

Three consequences worth writing down, because each is a way to get it
wrong:

- *The router matches on the `Host` header*, so a scrape aimed at
  `10.255.255.1:443` does not match it. The monitoring server resolves
  `api.repose.herakraft.co` to the tunnel address in `/etc/hosts` and
  scrapes the name, which keeps header, SNI and certificate correct;
  `prometheus.yml`'s `api` job is the name, not the address.
- *The allow-list is the only thing keeping `/metrics` off the
  internet*, since the router sits on the public entry point, and it
  matches the source address **Traefik sees**. That should be the
  monitoring peer's `10.255.0.x` over WireGuard, but it is unverified
  until the peer exists, and the failure mode is silent and open rather
  than loud and closed. `curl https://api.repose.herakraft.co/metrics`
  from anywhere else must answer 403; that check is part of closing 10's
  scrape row, not an optional extra.
- *The edge must forward 443 to the control plane*, not 9103, so
  `repose.edge.monitoring.scrapePorts` drops 9103 and gains 443. An edge
  built before this change would admit a port nothing listens on and
  refuse the one that answers.

Interfaces: none. `ops/coolify/README.md` holds the label block as the
documented step — a manual paste, which is the one thing here that I-87
would rather have in a file, so the block in the repository stays the
source of truth and a label that drifts from it is a bug.

**I-136. The api's user listener does not serve `/metrics`; the metrics
listener is the only place the registry is served.** (m5-release, 14 final
review, 2026-09-21) `internal/api/http.Server.New` mounted `GET /metrics`
on the user mux beside `/healthz` and `/readyz`, and the user mux is what
Coolify's proxy fronts on `api.repose.herakraft.co` with a `PathPrefix(/)`
router. So `curl https://api.repose.herakraft.co/metrics` answered 200 to
the internet: verified 2026-09-21 02:05Z from the dev box, from the edge's
public address and from the control VM, 57 `repose_*` series plus the Go
runtime's, no token asked. `docs/ops/OBSERVABILITY.md` promises "nothing
is reachable from the internet: every scrape and ship goes over the
edge's WireGuard", and I-133 built its whole argument on the allow-list
being "the only thing keeping `/metrics` off the internet" while the
application itself was serving it on the public port underneath. The
mount is gone; `MetricsHandler` on `API_METRICS_LISTEN` (`:9103`) is the
one metrics endpoint, which is also the port I-133's router already
points at (`loadbalancer.server.port=9103`), so the router keeps working
and the allow-list becomes defence in depth rather than the only gate.
`TestMetricsIsNotOnTheUserListener` pins it. What the series exposed:
counts of hosts, guests by state, ops, schedule results, secrets
operations, build failures, notification deliveries, rate-limit hits; no
tenant identifier (the registry refuses those labels, I-52) but a live
picture of the platform's size and activity, and an unauthenticated
handler on the public entry point that any scanner can hammer.
*Rejected:* keeping the mount and relying on I-133's `ipallowlist`
router (a label pasted by hand into Coolify, absent on the live app as of
this review; a defence that has to be present to work is not the layer
the application should depend on); a bearer check on the user-mux
`/metrics` (a second auth path for one endpoint that already has its own
listener). Interfaces: none (`api.md` never listed `/metrics`);
`05-control-plane-api.md` §5.1 corrected.

**I-137. hostd writes `host.json` and nothing else at registration; the
second `wg0.conf` under its state directory is gone.** (m5-release, 14
final review, 2026-09-21; closes review M-5) I-18 made `host.json` the
host's only runtime network input, rendered into `/run/repose/wg0.conf`
by `repose-host-net`; `internal/hostd/register.Register` still wrote
`/var/lib/repose/hostd/wg0.conf` and restarted `wg-quick-wg0.service`
itself, so a registration had two writers of one tunnel and a second copy
of the WireGuard private key on the persistent disk. Seen on host-01 as
deployed: `/var/lib/repose/hostd/wg0.conf`, 0600 root, dated the
registration (2026-09-20 18:15Z), unread by any unit. `writeWG`, the
`WGFile` constant and `Config.{Runner, WGUnit}` are removed; `WGConf`
stays as the renderer tests and operators compare against; `--no-wg` is
accepted for one release and ignored (its only effect was to skip the
restart that no longer happens; the `repose-host-net` restart after a
self-registration stays, per I-40). A host registered before this carries
the stale file until its next switch; it is inert, and the operator may
delete it. *Rejected:* keeping the file as a fallback for a host without
`repose-host-net` (every host has it; a fallback nobody runs is a second
truth).

**I-138. A project without a remote syncs its whole tracked tree and
commits it in the guest.** (m5-release, 07, 2026-09-21; review M5-9)
`repose run --name X` in a directory with no git remote, the case
`cli-config.md` gives `--name` for, created and booted its guest and then
failed at step 5c with `fatal: 'origin' does not appear to be a git
repository`: guestd sets `origin` only from the project's `remote_url`
(I-107) and the sync fetched it regardless. With `remote_url` empty the
sync now skips the fetch, the push prompt and the checkout; the tracked
files travel as a tar of their working-tree contents, are `git add`ed and
committed in the guest under a placeholder identity
(`repose <repose@localhost>`, a commit that exists only in the guest since
there is no remote it could reach), and the untracked files follow as
before. The commit is what keeps the guest tree clean, so the next run's
dirty-tree check still means what it means for every other project. A
file deleted on the laptop is not deleted in the guest: with no remote
there is no commit to derive the deletion from, and `git clean` against
an agent's tree is the one thing the sync must never do. *Rejected:* a
diff against the empty tree with `git apply --index` (a second run fails
on "already exists in index" unless the index and tree are cleared first,
which is the `git clean` above); leaving the staged files uncommitted (the
next run's dirty check refuses its own previous sync); refusing `--name`
without a remote (the flag exists for that directory). Interfaces: none;
`07-cli.md` §5.5f and `features/sync-at-launch.md` describe it.
*Superseded by I-150 (2026-09-23): the laptop's commits now travel as a
bundle for every project, remote or not.*

**I-139. `RegisterResponse` carries the SSH Host CA's public key; hostd
writes it to `host.json` and re-renders the host's network files after a
rotate that changes it.** (m5-release, 14 final review, closes review
M-1, 2026-09-21) `host-conventions.md` has documented `host_ca_pub` in
`host.json` since workstream 01, `repose-host-net` renders it into
`/run/repose/host_ca.pub`, sshd's `TrustedUserCAKeys` points there and
`repose-admin operator-cert` signs certificates for it; nothing ever sent
it. On host-01 as deployed the file is 0 bytes and every operator login
(750 in 24 hours) is by the bootstrap key in
`/etc/ssh/authorized_keys.d/root`, a static key with no serial in the
audit line, which is what `docs/SECURITY.md` "Not mitigated" recorded as
M-1. `RegisterResponse.host_ca_pub = 8` now carries `ca.HostCAPub()` from
the api (the same line `GET /internal/ca` and `POST /certs` already give
the gateway and the CLI), on `Register` and on `Rotate`; hostdev sends
its own SSH CA (the one it signs guest host keys and operator user
certificates with, review L-3); hostd's `write` keeps the previous value
when the field is empty, the same rule as `loki_url`, so an api that
predates the field changes nothing. Because the renderer runs at boot
and after registration only, the rotation loop now restarts
`repose-host-net` when a rotate changed `host_ca_pub` or `loki_url`
(I-95 said `Rotate` carries the Loki and never said how it reached the
file). What this does not do: remove the bootstrap key. host-01 keeps
`repose.host.bootstrap.enable` for the token and reinstall path (I-92);
"only with a certificate" (14 §9) needs that turned off once operators
hold certificates, which is 01/11's row. host-01 itself learns the CA at
its first rotate (about 2026-10-15) or by the runbook's by-hand step
("Operator certificate refused by a host"), taken at the host switch
that carries this change. *Rejected:* a separate unary RPC to fetch the
CA (a second round trip for one line that registration already answers);
delivering it in `Hello`'s ack on the stream (the stream is commands and
results; identity material travels on the unary path with the
certificate); hostd polling `/internal/ca` (hostd holds no gateway client
certificate). Interfaces: `grpc-hostd.md`, `host-conventions.md`,
`hostd.proto` (old shape accepted: field 8 is additive).

**I-140. Operator SSH logins reach `audit_log` as an `operator_login` host
event carrying the certificate's key id and serial, never its body.**
(m5-release, 14 final review, closes review L-13 and L-7, 2026-09-21) 14
§5 lists "every operator SSH login to a host or the edge" among the
audited actions; on host-01 as deployed the PAM hook wrote a journal line
(`pam_type`, `user_present`) and nothing reached Postgres: 750 accepted
logins in 24 hours, zero rows. And the line could not say which
certificate logged in (L-7). Now `hostd audit-login` parses sshd's
`SSH_AUTH_INFO_0` with `x/crypto/ssh`, keeps a certificate's `KeyId` and
`Serial` and the SHA256 fingerprint of the key (a plain key gives the
fingerprint alone, which is how the bootstrap key shows up), logs them,
and on `open_session` posts them to the daemon's control socket
(`POST /operator-login`, a 2 s timeout, failure logged and the login
never blocked); the daemon emits `Event.operator_login = 14` on the
stream with the usual event id and ack; the api's ingest writes
`audit_log (actor = key_id | "operator:key:" + fingerprint, action =
operator_login, target = host id, detail = {pam_type, user_present,
key_id, serial, key_fingerprint, host_event_id, ts})` and refuses a
second row for the same host event id, since a host re-sends an event
whose ack was lost. `PAM_RHOST` is read by nothing: the source address
is on the never-log list and an operator's address is not the audit's
business. *Rejected:* the api tailing the host's journal through Loki (a
log store is not an audit store, and no Loki is wired); writing the row
from the hook directly (the hook has no database and no api client, and
must never block a login on either); a separate unary RPC (the stream's
event path already has ids, acks and a re-send buffer). Interfaces:
`grpc-hostd.md` (Event kinds), `host-conventions.md` (`hostd
audit-login`), `hostd.proto` (old shape accepted: an api that predates
the kind ignores it and acks).

**I-141. A security sweep is due while the release is newer than this
process's last sweep.** (m3 integration, 2026-09-21) `basebump.Run`
checked every ten minutes for a security base released in the last ten
minutes. api-grpc is redeployed on every push to main, and a redeploy
inside that window restarts the ticker, so the release's only chance came
after its ten minutes were up and it waited for 04:00 UTC: the LTS
republish 2026.09.21.3 (02:32:36Z) met the 02:39:58Z rollout on host-01
and no project was rebuilt. The tick now sweeps when the newest base is a
security release newer than the last sweep this process ran, so a fresh
process sweeps it at its first tick and an old one does not repeat a
sweep it already made; a project already on the base is not touched by
either. `TestSecurityDueSurvivesRestart`. *Rejected:* a sweep at start
(a rollout of two replicas would race for the lock for nothing, and the
ten-minute tick is what `repose-admin base publish` promises: "unheld
projects rebuild at the next security sweep (within 10 minutes)"); recording the last
sweep in Postgres (a second replica's sweep would then hide a restart of
the first, which is fine, but the row is more state for a decision one
query answers).

**I-142. `host_moved` is raised only when a restore leaves the project's
host.** (m3 integration, 2026-09-21) The restore phase notified
`host_moved` ("was restored onto a new host") for every restore, including
`repose snapshots restore` over the stopped project's own volume on the
same host; on host-01 the three restores of 2026-09-21 (01:26, 01:57,
02:02Z) each sent the owner of m3-check that message while the project
never left host-01. `buildRestore` now records `host_moved` in the op's
params once it has picked the host (the project's when ready, else the
scheduler's), fixed on the first build like `new_guest_id` so a re-sent
command says the same; the restore result raises the event only when that
is true, and logs `restored` otherwise. `TestRestoreEmitsHostMovedEvent`
covers both directions (a same-host restore, then a project whose host
row is unreachable). *Rejected:* a `restored` notification kind for the
same-host case (the user asked for the restore and the CLI reports it; 13
§5's kinds are for what happens without them).

**I-143. The system activation leaves guestd running; guestd restarts
itself after a switch.** (m3 integration, 2026-09-21) A switch is run by
guestd. With the guestd unit at NixOS defaults, the new system's
activation stopped guestd whenever its binary had changed, and because
`switch-to-configuration` was guestd's child the stop killed it before
guestd's start step: the 2026.09.21.3 sweep on host-01 (02:59Z) left
age-calculator, m3-iso-c and m3-held with no guestd, `/run/current-system`
on the old system, the op `guest_unresponsive` (`guestd Switch: vsockrpc:
EOF`) and every later start re-applying the same revision with the same
result. `nix/guest/base/guestd.nix` sets `restartIfChanged = false` and
`stopIfChanged = false`, so the activation of any base from here on never
touches guestd, whichever guestd runs it: that is what makes the fix reach
the stranded guests, since it is the new closure's activation that
decides. guestd then compares the new system's `guestd.service` ExecStart
with its own binary and, when they differ, schedules `systemctl restart
guestd` 3 s later through `systemd-run --on-active`, after its Switch
result has reached hostd; hostd logs `guestd_lost` then `guestd_regained`.
The activation itself runs through `systemd-run --wait --pipe` as a
transient unit, so no stop of guestd can kill a switch again.
`TestSwitchAppliesWithoutReboot` (the wrapper, no restart for the same
binary) and `TestSwitchSchedulesGuestdRestartWhenItsBinaryChanged`.
*Rejected:* `KillMode=process` on guestd's unit (it is the old unit's
KillMode that applies, so it would not have helped the guests already
running); hostd re-checking the guest after an EOF (guestd was stopped,
not restarting, so there was nothing to wait for).

**I-144. One clone per base ref.** (m3 integration, 2026-09-21) Two
builds that needed the same new base at once each cloned into
`<ref>.tmp`; the second clone's pack landed in the first's directory and
one of them failed with `fatal: fetch-pack: invalid index-pack output`
(nuru-playground's bump onto 2026.09.21.3, 02:59:18Z, its revision
`failed`). `ensureBase` is serialized on the builder; the second build
finds the first's checkout. `TestEnsureBaseClonesOnceUnderConcurrency`.
*Rejected:* a per-ref lock (a map to maintain for a lock held for the
seconds of a clone a few times a week).

**I-145. A bump that built but could not switch says so.** (m3
integration, 2026-09-21) `base_update_failed` read "base X failed to
build" for every failed bump op; three of the 02:59Z sweep's had built and
failed at the switch. When the revision row is `built` the summary is
"base X built, but switching the running guest to it failed; the guest
keeps its current system", then the op's message.
`TestBumpSwitchFailureSaysSwitch`.

**I-146. A bump that failed against an older base is tried again on the
next.** (m3 integration, 2026-09-21) The sweep skipped every project whose
newest revision was `failed`, meant for a fragment the user has to fix
first. A bump can fail for the platform's reasons (I-144's clone), and the
rule then held the project on its old base until the user applied
something by hand. The skip now applies only when the failed revision is
on the latest base already; a newer base is a new attempt.
`TestSweepBuildsUnheldSkipsHeld` (2026.09.30 after the failed
2026.09.29). A fragment that is really broken fails again on the next
base and the user gets one `base_update_failed` per base, which is the
right amount of noise.

**I-147. A start applies only a built revision newer than the one the
guest runs.** (m3 integration, 2026-09-21) `buildApply` picked the newest
`built` row with a closure, and the http start route asked whether any
such row existed. A bump that ends `built` and is later superseded by an
applied newer revision leaves that row `built` forever; m3-held's start
at 04:38Z (its 2026.09.21.3 bump `built` with a reboot pending, then
2026.09.21.4 applied in place) applied the .3 closure over the running .4,
and that older base's pre-I-143 activation stopped guestd. Both now
select the newest built row created after the project's current revision
(`ops.PendingRevision`); a superseded row is not pending. *Rejected:*
marking superseded rows `stale` (a fourth status for what a timestamp
comparison says).

**I-148. The activation's output goes to a file, and hostd asks again
once when guestd went away mid-switch.** (m3 integration, 2026-09-21)
I-143 ran the activation as a transient unit with `systemd-run --pipe`;
when an older base's activation stopped guestd (I-147's case) the pipe's
reader was gone and the activation died of SIGPIPE before its start step,
which is the strand again by another route. The unit now appends to
`/run/repose/switch.log`, which guestd reads after `--wait`; nothing the
activation does to guestd can end it. On hostd's side a transport error
from Switch (EOF, not a remote error) waits for the guest's next session,
up to `guestd_lost_after` plus 30 s, and sends the same Switch once more:
the profile is set and the activation of the same system is a no-op, so
the retry is idempotent, and an activation that restarted guestd ends as
`done` instead of `guest_unresponsive`. `TestSwitchAppliesWithoutReboot`
(the log file, no pipe) and hostd's `TestBuildAndApply` (the fake guestd
dropping the first Switch, answered on the second connection).

**I-156. A destroy the user asked for always finishes; DELETE answers
with the op to wait on.** (server-robust, 2026-09-23) age-calculator's
guestd died in the 2026-09-21 base switch while its unit kept running;
each `repose destroy` (2026-09-22 02:05Z, 2026-09-23 00:03Z) failed at
step 0 with `guest_unresponsive` because the final Snapshot needs guestd's
Freeze, the project stayed in `error` billing its disk, and the CLI had
already printed "Destroyed." from the 202. The ops engine now has one
recovery per op (`ops.recoverFrom`, recorded in the op's params as
`recovered`): a destroy whose snapshot or stop fails `guest_unresponsive`
is replanned from that step as stop (no snapshot; hostd's stop falls back
to the hypervisor's shutdown and then to killing the unit, none of which
needs guestd), snapshot of the stopped volume (crash-consistent at worst,
reason `stop`, expiring in 30 days like any destroy snapshot), destroy.
If that snapshot still cannot be taken it is skipped with a
`snapshot_failed` event saying the newest earlier snapshot is kept, and
the destroy goes on. A `not_found` from the host in any destroy phase
means the guest is already gone and the op is done. `error` remains for a
real host failure (lvremove, the blob upload), with the host's code, and a
later destroy resumes: the stopped guest is snapshotted and removed.
`DELETE /projects/:id` answers `202 {op_id, state}` (the `state` field is
new; `op_id` was already there), and a DELETE while a destroy op is open
answers with that op instead of `409`, so a client that lost the first
response can still wait. hostd is unchanged: every step above uses
commands and states the running hostd already has. Healthy guests take
the old path (freeze, snapshot, stop). `TestDestroyWithDeadGuestdReachesDone`
(from `error` and from `running`), `TestDestroySkipsAnImpossibleFinalSnapshot`,
`TestDestroyOfAGuestTheHostLostIsDone`, `TestDestroyAndRestartContract`.
*Rejected:* always stopping before the destroy snapshot (it changes the
healthy path for a case the fallback covers); a hostd change to freeze-or-
fall-back inside Snapshot (needs a host switch, and a snapshot that
silently stops being clean is worse than an op that says so).

**I-157. `repose start` on a project in `error`, or on a running one
whose guestd stopped answering, restarts it onto its newest built
revision.** (server-robust, 2026-09-23) age-calculator's start sent
StartGuest (ok: the unit was running) then ApplyConfig (guestd dead),
four times in two minutes; only `repose-admin projects restart` could
bring it back. The start route now enqueues `ops.PlanRestart` for
`error`, and for `running` when the newest meter sample (under five
minutes old) says `guestd_ok = false`: stop without a snapshot, apply the
pending revision (I-147's `PendingRevision`) to the stopped guest, which
hostd does by moving its GC root without guestd, then StartGuest, which
boots that closure. The response is `{op_id, restart}`. A start op whose
ApplyConfig fails `guest_unresponsive` (the I-143 strand on the old plan)
turns into the same restart once. Booting on the new closure instead of
switching into it is what makes this safe for I-143's case: the
activation that stopped guestd is not run. `TestStartFromErrorRestarts`,
`TestStartWhoseApplyLosesGuestdRestarts`, `TestDestroyAndRestartContract`.
*Rejected:* a separate `restart` op kind or route (the op kinds are
fixed, and the user's recovery command should be the one they already
type); detecting a dead guestd from `guestd_lost` warnings (the sample's
`guestd_ok` is already stored per project).

**I-158. Stop, resize and snapshot on a dead guestd.** (server-robust,
2026-09-23) The same "fails instantly for ever" check across the other
ops. Stop with a snapshot sent StopGuest{snapshot_first}, whose freeze
needs guestd: it now recovers as stop without a snapshot, then a snapshot
of the stopped volume (reason `stop`); if that snapshot fails, the
project stays `stopped` with `last_error` and a `snapshot_failed` event
instead of going to `error`. Resize extends the volume and tells the
hypervisor before GrowFs needs guestd, and a retry repeats the same
failure; manual snapshots need a freeze. Neither can succeed without
guestd and neither should reboot the user's guest unasked, so both end
`error` with a message naming `repose start`, and resize's retry after
the restart completes (hostd skips the lvextend already done).
`TestStopWithDeadGuestdStopsThenSnapshots`,
`TestUnresponsiveErrorsAreSentences`. Not fixed: nothing grows the guest
filesystem after a resize of a stopped guest (no boot-time growfs in
`nix/guest`); that predates this and is recorded here for workstream 02.

**I-159. An op's error message is a sentence; the host's wording is
`detail`.** (server-robust, 2026-09-23) `op.error.message` and
`projects.last_error` carried hostd's text verbatim, e.g. `guestd
unreachable for guest 01a0c161-…`, a guest id the user never sees
anywhere else. `failWithLine` now maps the codes users meet to a sentence
that says what to do (`guest_unresponsive`: "the environment's agent
(guestd) stopped answering; `repose start` restarts it", with variants for
a boot that never answered, a resize and a snapshot; `host_unreachable`;
`insufficient_capacity`), keeps `code`, and moves the host's message to
`error.detail` for operators. Other codes keep their message. Logs are
unchanged: the op-failure line carries the code, never either message.
`TestUnresponsiveErrorsAreSentences`.
**I-160. A create reuses a closure the host already runs.** (provision-speed,
2026-09-23) The owner's `izma` create spent 6.1 s in `Build` (eval 5.7 s,
build 0.33 s, 6.2 s CPU, 872 MB peak) to produce the closure four guests on
host-01 were already running: the fragment was the default one and the
base the latest. The system closure depends only on the fragment's text,
the base ref and the base version label (I-34, I-43; checked in
`nix/flake.nix`: `guestSystem` reads nothing else), so the create's
`build` phase now looks for the applied revision of another live project
on the chosen host (not destroyed, not `destroying` or `error`, guest id
set) with the same fragment on the same published base version; when one
exists the revision becomes `built` with that closure and
`kernel_changed = false`, no `Build` is sent, and `CreateGuest` follows in
the same engine pass. That guest's GC root (`guests/<id>`, removed only by
`DestroyGuest`) keeps the path on the host. Only published bases qualify:
`dev` names the api's `--base-ref`, which can move. A create whose
fragment matches no running closure, every restore and every
`config apply` still build. The build phase also records `base_version` on a create's revision
now (the `status <> 'building'` guard skipped that update for every create,
so those rows had none; the reuse query reads the project's base for
them). Saves the whole `Build` phase (5–6 s on a warm host, more on a cold
one) for most creates. `TestCreateReusesAClosureOnTheSameHost`.
*Rejected:* a per-host cache keyed by a fragment hash (a table to keep in
step with GC for what one indexed join answers), and sharing across hosts
(the path would have to be copied, which is a build's cost again).

**I-161. The guest boot's critical chain: no wait for Docker, the console or
a mount rate limit.** (provision-speed,
2026-09-23) m3-check's boot on host-01 (base 2026.09.21.x, small, a start):
Cloud Hypervisor started 02:31:55.24, hostd `running` 02:32:09.38, 14.1 s.
In the guest: kernel 0.84 s, initrd 4.21 s, then `docker.service`
8.66 → 11.04 s, and guestd (`After=docker.service`), sshd (`After=guestd`)
and everything behind them waited for it; after guestd started, Ready came
at the second 0.5 s poll of `/proc/net/tcp` (11.69 s), `RegisterPaths`
at 12.14 s, `repose-paths` saw its stamp at its next 0.5 s poll (12.65 s),
home-manager ran 1.07 s, `systemd-user-sessions` lifted `/run/nologin` at
13.79 s and SetupProject's `systemctl --user -M dev@` (a login session)
returned at 13.85 s, which is when hostd says `running` and when an SSH
login is first accepted. Nothing guestd does at boot needs Docker, so
guestd is now ordered only after tmpfiles and `network.target`; Docker
starts in parallel. guestd's `docker_down` warning waits 60 s from its
start (`sample.DockerGrace`) unless the socket has answered once, so a
Docker still starting is not reported as down. The Ready poll is 0.1 s
(`guestd.ReadyPollInterval`) and so is `repose-paths`' wait: both sit on
the path to the first login.

Two silent waits in the same boot were found by booting the guest runner
with Cloud Hypervisor on the dev box (unprivileged, virtiofsd
`--sandbox none`, `systemd.log_level=debug`; its untouched baseline
matched host-01 within 0.1 s): systemd queried the serial console's size
and terminfo and waited out the timeout, 0.67 s in the initrd and 0.33 s in
stage 2, and the initrd services' credential mounts tripped systemd's
mount-monitor rate limit, which held `sysroot.mount` and the store mount
back for about 0.7 s. The base now passes
`systemd.tty.{term,rows,columns}.console` on the kernel line (all three are
needed) and clears `ImportCredential` on the initrd's services (a drop-in
for the upstream `systemd-fsck-root` and `systemd-tmpfiles-setup-sysroot`;
without it the initrd refused the unit). On the dev box, CH start to
guestd Ready went from 11.88 s to 6.22 s and 6.39 s with this commit's
runner (`nix build ./nix#guest-runner`), about 5.5 s off every boot
(create, start, restore, reboot); on host-01 that is expected to take
CH-to-`running` from 14 s to about 8.5 s, confirmed only after the next
base publish. Not adopted: virtiofsd `--cache always` (hostd) took another
0.9 s off on the dev box, but an in-place `ApplyConfig` needs the guest to
see store paths that appear after boot, which that mode's long-lived
dentry cache is not known to do; it needs its own test first. An
uncompressed initrd changed nothing. `TestDockerDownWarnsOnce` (no warning inside the
grace), `TestDockerDownAfterItAnsweredWarnsInsideTheGrace`.
*Rejected:* keeping guestd after Docker and moving only sshd
(hostd's `running` waits for guestd's Ready either way).

**I-162. mkfs leaves the inode tables to the guest's lazy init.**
(provision-speed, 2026-09-23) izma's CreateGuest on host-01 spent 1.50 s
between `creating` (00:06:54.05) and `starting` (00:06:55.56), against
0.13 s for m3-check's StartGuest (02:31:55.10 → 55.23), which does the same
steps without the volume. `lvs` answers in 24 ms and `blkid` in 2 ms there,
so the difference is `mkfs.ext4 -E lazy_itable_init=0`: the thin volumes
report `write_zeroes_max_bytes` 0, so mke2fs writes the inode tables as
real zeros, about 670 MB for a 40 GB volume (izma's volume was 2.23 percent
allocated right after the create, the 20 GB ones 0.8 percent), at the
disk's 600 MB/s. hostd now runs `mkfs.ext4 -E lazy_itable_init=1`; the
guest kernel's ext4lazyinit zeroes the tables in the background at its own
low rate, and unprovisioned thin blocks read as zeros meanwhile. Expected:
about 1 s off every create (not start) and no 670 MB write burst on a disk
every guest on the host shares. Needs a host switch.
`internal/hostd/lvm` test pins the argv. *Rejected:* `noinit_itable` in the
guest's mount options (never zeroing is safe on thin but is one more
guest-visible difference for no time the create would see).

**I-163. An op enqueued in one api process wakes the driver in the other
through NOTIFY.** (provision-speed, 2026-09-23) The api and api-grpc each
run an ops engine and only the one holding the ops lock drives; `Kick` only
woke its own process. On 2026-09-23 api-grpc drove: izma's Build result to
the CreateGuest send took 45 ms (the result lands in api-grpc and kicks
locally), but `POST /projects` is served by the api, so the op waited for
api-grpc's 500 ms poll before its Build went out (the api logged the POST
and api-grpc the command in the same second; hostd started the Build at
00:06:47.86). A `Kick` in an engine that is not driving now also runs
`select pg_notify('repose_ops', '')`, and the driver `LISTEN`s on a
dedicated connection and turns each notification into a tick. The 500 ms
poll stays as the fallback, so a lost listener or a failed notify costs
latency, never an op. Expected: up to 0.5 s (0.25 s on average) off the
start of every op a user or `repose-admin` enqueues, and the same off each
phase when the api rather than api-grpc holds the lock.
`TestEnqueueInAnotherProcessWakesTheDriver` (driver polling once an hour,
the create done 1.8 s after the other engine's kick). *Rejected:* a
shorter poll (four times the queries for half the gain) and routing the
enqueue over the internal gRPC hop (a new RPC for what Postgres already
carries).

**I-149. The CLI has its own passphrase-less key, and one SSH connection
per command.** (cli-ux, 07, 2026-09-23; owner's v0.1.4 session) The
certificate was issued for `~/.ssh/id_ed25519`. The owner's is
passphrase-protected and not in an agent, and every `runSSH` opened a new
connection, so one `repose run` asked for the passphrase four or five
times and the gateway closed the connection while a prompt waited
(`Connection closed by … port 22`, exit 255). The CLI now generates
`~/.ssh/repose/id_ed25519` in process (ed25519, no passphrase, 0600; no
`ssh-keygen` needed) and the certificate is issued for it; a certificate
on disk for any other key is re-issued, so a v0.1.4 laptop moves over on
its next command. The user's own keys are never read, written or
offered, and nothing is `ssh-add`ed. The generated config points
`IdentityFile` at the new key, keeps `IdentitiesOnly yes` (I-108), and
adds `ControlMaster auto`, `ControlPath ~/.ssh/repose/cm-%C`,
`ControlPersist 10m` (not on Windows, whose OpenSSH has no
multiplexing): the first ssh of a command opens the connection and every
later one (probe, sync, tmux, attach) is a session on it, one handshake
per `repose run`. The persisted master counts as an open gateway session
for up to ten minutes after the command ends; nothing acts on that count
today. `runSSH` bounds the wait for ssh's pipes (`WaitDelay`) so an old
client whose master held them could not hang the CLI. A connection the
gateway refuses (`Permission denied`, a revoked certificate) gets one
forced re-issue before the 60-second wait continues (the HANDOFF finding
"no certificate re-issue on certificate revoked").
`TestEnsureCertMovesOffTheUsersKey`, `TestRenderSSHConfigGolden`,
`TestSyncOverAMultiplexedConnection` (one TCP connection seen by the fake
guest for the whole sync). *Rejected:* asking for the passphrase once
and loading the key into an agent (needs an agent, and the key would
still be offered elsewhere); `ssh-add` of the certificate (the old
behaviour, which prompted too). Interfaces: `ssh-gateway.md` "CLI side"
and `cli-config.md` in this commit.

**I-150. The laptop sends its commits to the guest; the guest never
fetches origin during a sync.** (cli-ux, 07, 2026-09-23; owner's session)
Step 5c ran `git fetch origin` in the guest, which has no credentials for
a private repository (and none for a public one with an SSH remote,
which is what guestd sets, I-107): `git@github.com: Permission denied
(publickey)`, and izma's checkout stayed an empty `git init`. The sync is
now two ssh round trips on the multiplexed connection. The first creates
the checkout if missing, and reports the dirty list, the commits every
guest ref points at, and whether `origin` exists. The laptop filters
those commits to the ones it has, and `git bundle create`s `HEAD` and its
own `refs/remotes/origin/<branch>` excluding them (the whole history the
first time; nothing when the guest is current). The second carries one
tar (bundle, `git diff HEAD --binary`, the untracked tar) and one script:
stash or discard if asked, `git fetch` from the bundle, move
`origin/<branch>` to the laptop's view of it (only forward), add
`origin` if missing (guestd's URL rule), then check out: the branch is
created, or fast-forwarded when it is behind, and left alone with the
laptop's commit checked out detached when the guest's branch has commits
the laptop lacks (an agent's work is never moved off its branch), with a
warning saying so; then the diff and the untracked files as before. The
push prompt of 5.5c is gone: the commit travels whether or not it was
pushed, and nothing is pushed. A `--name` project with no remote uses the
same path without the remote-tracking ref, which replaces I-138's
placeholder-author commit and makes a laptop deletion a guest deletion (a
guest synced the I-138 way has a placeholder commit on its branch, so its
first sync this way checks out detached and says so). A laptop checkout
that is not a repository, has no commit, or is shallow gets a sentence
saying what to run. Credentials sync first, in one ssh: the allowlisted
files, the git identity through `git config --global` (from files in the
payload, not the command line), and, when gh's login travelled and the
remote is on github.com, `url.https://github.com/.insteadOf
git@github.com:` plus gh as the credential helper for github, so an
agent's `git push` works. gh 2.40+ keeps its token in the laptop keyring
and `hosts.yml` has none; the CLI then writes `gh auth token`'s value
under `github.com:` in the copy that travels (the same login, copied over
SSH: the second of the three homes for secrets).
`TestSyncSendsAnUnpushedCommit`, `TestSyncIntoAnEmptyGuestRepo`,
`TestSyncLeavesADivergedGuestBranchAlone`, `TestSyncNoRemoteSendsHistory`,
`TestSyncCredentialsCopiesExactlyTheFourRows`,
`TestSyncCredentialsCarriesAKeyringGhToken`. *Rejected:* forwarding the
laptop's agent for the fetch (ForwardAgent stays, but the owner had no
agent, and HTTPS remotes need a token anyway); pushing for the user (the
v0.1.4 prompt: it publishes work the user did not ask to publish, and
fails the same way when the push is refused). Interfaces:
`guest-conventions.md` (the `.gitconfig` row) in this commit.

**I-151. The CLI proves the `<slug>.repose` alias works and says exactly
how to fix it when not.** (cli-ux, 07, 2026-09-23; HANDOFF finding) On
the owner's laptop `ssh age-calculator.repose` did not resolve after the
Include line was written. `ensureIncludeLine` only checked that the text
appeared somewhere; an `Include` after a `Host` or `Match` line applies
to that block only, and a symlinked `~/.ssh/config` (home-manager) was
replaced by a regular file. Now an Include counts only before the first
Host/Match line (a fresh one is prepended otherwise, the misplaced one
left where the user put it), a symlinked config is edited at its target
when writable and never replaced, and after writing, `ssh -G
<slug>.repose` must resolve to the gateway host and the `<slug>.<handle>`
user. When it does not, the command warns with the reason and the line
to add (for a read-only link, where to add it, with the home-manager
option), and the CLI's own connections use `ssh -F ~/.ssh/repose/config`
so the run still works. An unwritable `~/.ssh/config` is therefore a
warning, not the exit 1 of 07-cli.md §6. `TestIncludeEffective`,
`TestEnsureIncludeLineLeavesAReadOnlyLinkAlone`.

**I-152. A directory's cached project must share its remote, and naming
a project never writes the directory cache.** (cli-ux, 07, 2026-09-23;
owner's session) In the nuru-wasm checkout `repose run` and `attach`
went to age-calculator: `resolveProject` trusted `by_dir[cwd]`, and every
explicit `--project` wrote `by_dir[cwd]`. Now an explicit project
(positional, `--project`, `$REPOSE_PROJECT`) writes nothing; `by_dir` is
written only when `run` creates a project with no remote (the case it
exists for, `cli-config.md`), keyed by the repository root so a
subdirectory finds it; and a `by_dir` or `by_remote` entry is believed
only when that project's `remote_url` equals the directory's normalised
remote (or both are empty). A mismatched or 404 entry is deleted from
`projects.json`, which cleans the entries v0.1.4 wrote.
`TestResolveProjectOrder` (poisoned entry ignored and forgotten, explicit
project writes nothing, root key).

**I-153. The CLI says what actually happened: the true state, why, and
the next command.** (cli-ux, 07, 2026-09-23; owner's session) `repose
destroy` printed "Destroyed." from the 202 while the op failed and the
project stayed; `attach` said "is stopped. Run `repose start`" for a
project in `error`; errors reached the user as `guestd unreachable for
guest 01a0…` or `ssh cd ~/izma && git status --porcelain: exit status
255: …`. Destroy now asks `Destroy <slug>? A final snapshot is kept for
30 days. [y/N]` (the owner's wording; typing the name added nothing
given the snapshot; `--yes`/`-y` skips it, and without a terminal the
CLI asks for `--yes` instead of assuming), waits on the op DELETE
returns (api.md, I-156), prints `Destroyed <slug>` only when it is done
and `GET` answers 404, and otherwise `Could not destroy <slug>: <reason>
(<code>). <slug> is still there, in state error. \`repose destroy
<slug>\` tries again.` An api without the op id is waited on by polling
the project. Every command that needs a running guest names the real
state with its own next step (exit 5 kept); an `error` project's reason
comes from the api's `last_error`, which since I-159 is a sentence and is
shown as is, while an older api's host wording is mapped by code and
never shows a guest id. Failed ops read `Could not <verb> <slug>:
<reason> (<code>). <next>`, with the host's `detail` only under `-v`; a
failed ssh reads `Could not <step>: <why> (<ssh's last line>).` and never
includes the remote command. `repose projects` has a header row, a dash
for what does not apply (uptime only while running; v0.1.4 printed 47h
for a project two days in `error`), and one line per errored project
with its reason; `--json` is unchanged. The `[y/N]` prompts now default
to no on an empty answer (v0.1.4's helper said yes to an empty answer
on `snapshots restore`'s `[y/N]`), and `secrets set` reads the value with
echo off. The destroy message names the project id for the restore,
since a destroyed project no longer resolves by name and the snapshot
commands accept that id. The api's `last_error`, `host_unreachable` and
`signals.guestd_ok` are read when present (they are in the api's Project
JSON, not yet in api.md's shape). `TestDestroyReportsAFailedOp`,
`TestDestroyConfirmationIsYesNo`, `TestNotRunningMessagesSayTheTruth`,
`TestProjectsTable`, `TestSSHErrorsAreSentences`.

**I-154. Long commands show live phases.** (cli-ux, 07, 2026-09-23;
owner's session) `repose run` on a new project sat silent for over a
minute. Commands that wait (run, start, stop, destroy, resize, snapshot
create and restore) now show the phase on stderr: on a terminal one
spinner line with the elapsed time, turned into `✓ <done>  <time>` when
a phase has its own result (Created, Built the environment, Booted), and
elsewhere one plain `<phase>...` line per phase; `run` ends with `Ready
in <time>.` The phase follows the project's state while an op runs
(creating, building, starting), a start the api turned into a restart
says `Restarting <slug> (its agent stopped answering)` (I-157), and the
build log streams through the same line without tearing it. Ops and the
project are polled every 500 ms (two cheap GETs), not 2 s; the build log
stream no longer inherits the api client's 30-second timeout, which cut
every longer build's log off, and a stream cut short resumes from its
last line. `--json` commands show nothing; Ctrl-C clears the line and
exits 130. `TestProgressOutput`, `TestStartFromErrorSaysRestarting`.

**I-155. A project is the argument of the commands whose object it is.**
(cli-ux, 07, 2026-09-23; owner's request, "like docker logs
<container-name>") `attach`, `start`, `stop`, `destroy`, `status`, `logs`
and `events` take `[PROJECT]`; `--project` and `$REPOSE_PROJECT` still
work, and naming two different projects is a usage error. `run` keeps
PROMPT as its argument (now everything after the flags, so quoting is
optional), and a one-word prompt that is exactly one of the user's
project slugs is refused with exit 2 and the right command (`repose run
izma` was almost certainly not a prompt; `--agent` sends it anyway).
`open`, `secrets`, `config` and `snapshots` keep `--project`, since their
argument is a port, a name, a path or a snapshot id. A stray argument on
a command that takes none is now a usage error (v0.1.4 ignored `repose
attach projects` and attached to the checkout's project), and cobra's own
refusals (unknown command, flag or arity) exit 2 instead of 1.
Completion offers the account's slugs for the argument and for
`--project` (the api with a two-second limit, else the cached slugs),
and fixed values for `--agent`, `--size`, `--kind`.
`TestPositionalProject`, `TestRunRefusesAPromptThatIsAProjectName`.

**I-164. A snapshot reads the blocks the filesystem uses, not the whole
volume.** (destroy-restore, 03, 2026-09-23; owner's `repose destroy izma`,
38 s) izma's destroy on host-01: StopGuest 02:22:46.48, `snapshot start`
the same millisecond, `snapshot done` 02:23:19.33 (`duration_ms` 32853,
`bytes` 2059646), stop 3.5 s, DestroyGuest 0.1 s. Nothing logged in the
33 s because nothing but one pipeline ran: `dd if=<snapshot> bs=4M |
zstd -T4 -3` reads every byte of the thin volume, and the unprovisioned
ones read as zeros that compress to nothing. The journal since
2026-09-21 shows the time follows the volume, not the data: every 20 GB
volume took 15.0-16.5 s (1.5-4.0 MB uploaded), every 40 GB one 30.9-34.8 s,
whether it carried 2 MB or 227 MB (02:26 and 02:31 the same night). The
freeze, `lvcreate -s`, the upload and the lvremove are each well under a
second. Every stop, nightly and destroy snapshot paid it, and every
restore paid it again writing 40 GB back through `dd conv=sparse`.
hostd now runs `dumpe2fs` on the activated snapshot and, when the
filesystem is ext4, `clean` and without `needs_recovery` (ext4's freeze
flushes the journal and clears that flag, and so does a clean shutdown),
reads only the ranges the block bitmaps mark used, drops 64 KiB pieces
that are all zero (the inode tables `lazy_itable_init=1` leaves unzeroed,
I-162), and frames the rest as `offset, length, bytes` records behind a
`RPSXT001` header and before a trailer with the byte count, through the
same `zstd -T4 -3` to the same blob path. Restore peeks the magic after
`zstd -d`: extent streams are `pwrite`n into the fresh volume (which reads
zeros everywhere else, the assumption `conv=sparse` already made) and
checked against the trailer, raw streams take the old `dd`. Anything the
bitmaps cannot be trusted for (a killed guest's journal, a volume that is
not ext4, dumpe2fs output that does not list every group) falls back to
the raw read, and the log line says which and why (`format`,
`raw_reason`, `used_bytes`, `volume_bytes` on `snapshot done`). On the dev
box a 40 GB sparse ext4 holding a 1 GB file: raw 23.6 s, extents 0.23 s,
same compressed size (`TestExtentSnapshotTiming`, a sparse file standing
in for a thin volume on an AMD box, so direction and scale, not host-01's
number). Expected on host-01: izma's snapshot from 33 s to well under a
second of reading plus the upload of what it holds; a stop from 38 s to
about 5 s. Needs a host switch; the api is unaffected. Old blobs restore
as before; an extent blob cannot be restored by a hostd older than this
(e2fsck fails that restore and only the new volume is touched), which
matters only once a second host exists. `TestExtentSnapshotRoundTrip`
(mkfs.ext4 -d, round trip onto an empty device, e2fsck -fn clean, files
equal via debugfs), `TestRawFallbacks` (not ext4; `needs_recovery` set
with debugfs), `TestParseDumpe2fsRefusals`,
`TestExtentStreamRefusesATruncatedOrOversizedStream`. *Rejected:* the
thin pool's own mapping (`thin_dump` needs `reserve_metadata_snap` on the
live pool every guest shares, and knows provisioned blocks, not used
ones); `e2image -ra` to a pipe (writes the zeros itself, so the time
stays proportional to the volume); keeping the LVM snapshot on the host
and exporting it later in the background (a host-side queue to reconcile
across restarts for seconds of upload nobody waits on once the CLI
returns at once, I-166, and a destroyed project whose only copy is on
one host is not restorable elsewhere until it lands).

**I-165. A destroy stops the guest first, reads `destroying` from the
moment it is accepted, and says so when it fails.** (destroy-restore, 05,
2026-09-23) izma's destroy froze the running guest, snapshotted it for
33 s (I-164) with the guest still running and billed, then stopped it.
`PlanDestroy` is now stop (no snapshot), snapshot of the stopped volume
(reason `stop`, 30-day expiry), destroy; a stopped project skips the
stop. Ending the guest first ends its hours and its sessions at once, and
the snapshot of a shut-down filesystem is clean without guestd's freeze,
so the dead-guestd case of I-156 is now the ordinary path (the recovery
stays for ops enqueued before this). `DELETE` sets the project to
`destroying` in the same transaction as the enqueue, and the engine keeps
it there through the stop and the snapshot (it used to read `stopping`,
then `running` again during the snapshot of a stopped project, and
`destroying` only for the last 0.1 s). A destroy that fails leaves the
project in `error` with `last_error` as before and now also records a
`destroy_failed` event, which notifies like `snapshot_failed`: the CLI no
longer waits for the destroy (I-166), so the failure has to reach the
user by itself. A destroy op enqueued before this release with the old
plan (stop, destroy) still snapshots inside its stop, because the stop
snapshots unless a snapshot phase follows. Timeline for izma, api-only
(before a host switch): DELETE → `destroying` at once; stop 3.5 s;
snapshot 33 s (the guest already down); destroy 0.1 s. With I-164 on the
host: stop 3.5 s, snapshot about 1 s plus the upload of what the volume
holds, destroy 0.1 s. `TestDestroyStopsFirstAndReportsItsFailure`,
`TestDestroyWithDeadGuestdReachesDone` (no recovery needed now),
`TestRestoreByName` (the state right after DELETE). *Rejected:* keeping
the freeze-then-stop order (the guest runs and bills through the whole
upload for a snapshot that is less clean); marking the project
`destroyed` right after the stop and finishing the snapshot and the
volume in the background (a project listed as destroyed whose final
snapshot may still fail is a promise the list cannot keep; I-164 makes the
remaining work seconds).

**I-166. `repose destroy` returns when the api has accepted the destroy.**
(destroy-restore, 07, 2026-09-23; owner: "perhaps we need to make it
background, but the issue is getting that restore command") v0.1.5 waited
the whole op (38 s for izma) to print the restore command. After the
`[y/N]` the CLI now sends `DELETE`, and on the `202` prints `Destroying
<slug>. Bring it back within 30 days with: repose restore <slug>` and
exits 0: one api round trip, well under the 1-2 s target. The restore
command no longer needs anything the op produces, because `repose
restore` resolves the name and the newest snapshot itself (I-167).
Failures stay visible without the wait: the project reads `destroying`
from the DELETE on (I-165), a failed destroy leaves it in `error` with a
reason whose next step is `repose destroy <slug>` (the api writes it into
`last_error`, and `repose projects`/`status` show it instead of the
generic "`repose start` restarts it"), and `destroy_failed` notifies.
`--wait` keeps the I-153 behaviour for scripts (wait on the op, then on
the 404, `Destroyed <slug> in <time>` only when gone, `Could not destroy`
and exit 1 otherwise) and its last line now names `repose restore <slug>`
too. Found on the way: `stop` and `destroy --wait` took the *last*
element of `GET /snapshots` as the newest, but the api lists newest first
(the fake oldest first), so a stop printed its project's oldest snapshot
id; both now pick the newest by `created_at` (`newestSnapshot`). The
"destroying/destroyed" not-running message names `repose restore` as
well. `TestDestroyConfirmationIsYesNo`, `TestDestroyThenRestoreByName`,
`TestProjectsShowAFailedDestroy`, `TestNewestSnapshotIgnoresOrder`,
`TestDestroyReportsAFailedOp` (now with `--wait`). *Rejected:* waiting
only for the stop and returning before the snapshot (still 4 s, and the
stop is not what the user cares about); a background process on the
laptop that waits and notifies (a laptop that closes loses it; the api
already notifies).

**I-167. Restore by name: `GET /projects/destroyed`, `POST
/projects/restore`, `repose restore NAME`.** (destroy-restore, 05/07/08,
2026-09-23; owner: "the restore command is too complicated") The destroy
message ended with `repose snapshots restore <snapshot id> --project
<project id> --as-new NAME`, because a destroyed project no longer
resolves by name. `GET /projects/destroyed` lists the user's destroyed
projects that still have a restorable snapshot (not deleted, not past
`expires_at`), newest destroy first, with that snapshot, its expiry as
`restorable_until`, and `name_free`. `POST /projects/restore {slug |
project_id | snapshot_id, name?, start?}` resolves a slug the way a user
means it (the live project with that slug if there is one, else every
destroyed project that had it), takes the newest restorable snapshot
unless one is named, and restores it as a new project called `name`,
default the source's name; a taken name is `409 conflict` with
`detail.reason = "name_taken"` so a client can ask for another, and a
project with nothing left is `404` with `detail.reason = "no_snapshot"`.
The new project also gets the source's `remote_url` when no live project
has it (the old as-new restore dropped it, so a checkout never found the
restored project), and `POST /projects/:id/snapshots/:sid/restore
{as_new_project}` now shares that code. Both routes are new, nothing old
changes shape. api.md also gains the Project fields the api has returned
since I-157/I-159 (`last_error`, `host_unreachable`, `signals.guestd_ok`)
and the restore route's `start` and `project_id`. The fake api has both
routes, `destroyed_at`-ordered, and the three fields.
`TestRestoreByName` (api, against Postgres), `TestWalkthrough` (fake).
*Rejected:* restoring in place when the name is live (that replaces a
running project's disk; `repose snapshots restore ID` still does it, with
its stop prompt); a `?destroyed=true` switch on `GET /projects` (one
route, two shapes); resolving by slug inside the existing snapshot route
(its path starts with a project id, which is what the user does not have).
CLI: `repose restore NAME [--as NEW-NAME] [--snapshot ID]` posts that
route (NAME may also be a project id, so the v0.1.5 destroy message's id
still works), waits with the create phases and ends `Restored <slug> from
its snapshot of <time> in <elapsed>; it is running (<class>). \`repose
attach <slug>\` to get in.` A name in use asks for another on a terminal
(empty cancels) and otherwise exits 2 naming `--as`; nothing to restore
exits 3 with the api's sentence and `repose projects --destroyed`, which
lists `PROJECT CLASS DESTROYED SNAPSHOT SIZE RESTORABLE UNTIL` (`--json`
is the api's list). Completion for `restore` offers the destroyed slugs
(the api, two seconds at most). `repose snapshots restore` is unchanged.
`TestDestroyThenRestoreByName` (fake api: destroy, list, restore by name
with the remote, live-name 404, name taken without and with a terminal,
unknown name). Not done: `repose restore` with no NAME inside a checkout
whose project was destroyed (the remote could find it; it asks for the
name instead).

**I-168. The dashboard lists recently destroyed projects with a Restore.**
(destroy-restore, 08, 2026-09-23; owner: "perhaps they can use the
dashboard?") `/projects` gains a "Recently destroyed" section under the
table, from `GET /projects/destroyed` (I-167): name, class, a `name in
use` badge, "Destroyed <relative time> · snapshot <time>, <size> ·
restorable until <date> (N days left)", and `Restore…`, which opens a
name field with the old name (or `<slug>-restored`, `-2`… when a live
project has it), posts `POST /projects/restore {project_id, name}`, and
goes to the new project's page, where its state shows the restore
running. A `name_taken` 409 shows under the field instead of a toast. The
section is a `form-section` with a `font-display` heading and `btn-ghost`
row actions, per DESIGN-LANGUAGE.md; nothing new in `layout.css`. The
section is asked for only after the live list loaded, because its
answer would otherwise clear the "cannot reach the api" bar that a
failed list had just raised (the failure-mode test caught this), and an
error from it (an api older than I-167) hides the section silently. A
project in `error` now shows its `last_error` sentence under its state
in the table, so a destroy that failed after the dashboard moved on is
visible there too, with the retry the api wrote (I-166). The destroy
toast says the destroy continues and where to restore from. Tests:
`tests/project-lifecycle.spec.ts` (destroy through the UI, the row with
its expiry, Restore under the same name, lands on the new project),
`src/lib/destroyed.test.ts`. *Rejected:* a separate `/projects/destroyed`
page (one more route for a list that is short and belongs next to the
live one); restoring on one click without a name field (it creates a
billed project, and the name may be taken).
