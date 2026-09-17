# Milestones

Each milestone is usable on its own and has a gate. Workstreams (see
`workstreams/README.md`) map onto milestones; several workstreams run in
parallel inside one milestone, and some start early because nothing blocks
them. "Done" for a milestone means every listed workstream's checklist is
closed and the gate is demonstrated, not that code exists.

## M0. Benchmark gate (deferred, DECISIONS I-12)

Skipped by the owner's choice on 2026-09-17. The first M1 host records real
timings instead; the procedure below remains for when a comparison is needed.

Workstreams: `00-benchmark`.

Provision one `D64s_v5` host with `--security-type Standard` by hand (az CLI
is fine here), install NixOS with nixos-anywhere from a throwaway host config,
boot one microvm.nix Cloud Hypervisor guest with the shared store, Docker
inside, 4 vCPU 8 GB. Measure against a plain `D4s_v5` Azure VM:

- CPU: `nix build` of a fixed derivation set, and a Go test suite, wall time.
- Disk: `docker pull` and extract of a fixed 2 GB image; `fio` 4k random rw.
- Network: `git clone` of a fixed 1 GB repo from GitHub; `iperf3` to the edge.

Gate: penalty under ~20 percent on each axis. Record all numbers, SKUs, kernel
and CH versions in `RESEARCH.md`. If the gate fails: repeat once on
`D64s_v6`; if that fails, open a decision to move hosts to Hetzner and
continue with M1 unchanged in design.

M0 can run in parallel with every M1 workstream that does not need a host.

## M1. Hosts, guests, and the daemon pair

Workstreams: `01-host-nixos`, `02-guest-base`, `03-hostd`, `04-guestd`,
`12-nix-config-pipeline`, `10-observability` (host and guest parts).

Gate: on one real host, a developer's own projects run as guests created by
gRPC calls from a local `hostd` client, with the shared store, Docker inside,
config apply in place, snapshot and restore round-trip, process samples and
metrics visible in Grafana. Reached over WireGuard from the laptop with a
manually issued certificate.

## M2. Edge, identity, CLI

Workstreams: `06-gateway-edge`, `05-control-plane-api` (auth, CA, projects,
routing), `07-cli`, `11-infra-opentofu` (edge, hosts, blob, kv).

Gate: a second person with a GitHub account runs `factory login` and `factory
run` on their laptop and lands in tmux in their own guest on the shared host,
cannot reach the first person's guest, and gets a notification when their
agent finishes.

## M3. Control plane on Coolify, dashboard, secrets, config menu

Workstreams: `05-control-plane-api` (rest), `08-dashboard`, `13-notifications`,
`11-infra-opentofu` (Coolify VM, R2), `14-security` (policy text, review).

Gate: the API and dashboard run on Coolify with rolling deploys, Postgres backs
up to R2 nightly and a restore has been rehearsed, secrets set in the
dashboard appear in a guest, a non-Nix user adds a package from the menu and
sees it in their guest without a reboot.

## M4. Billing

Workstreams: `09-billing`.

Gate: a real card is charged the right amount for a known usage pattern
(one large guest, 100 hours, 40 GB, 10 GB egress) and the Stripe invoice
matches the `usage` rows to the cent. Trial credit depletes and blocks a start
at zero. A failed payment stops guests after 3 days.

## M5. Public

Workstreams: `14-security` (final review), `ops` docs, launch checklist in
`CHECKLIST.md`.

Gate: `CHECKLIST.md` closed. Landing page at `factory.herakraft.co`.

## Later, in order of likely demand

Preview URLs. Idle auto-stop (needs M1 signals for a month). `factory mcp
forward`. Telegram and Discord. Hetzner host module. Per-project LUKS. Teams.
Central builder. Automatic capacity.
