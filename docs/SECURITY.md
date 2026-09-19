# Security

The threat model for repose, the boundaries that hold it up, the rules that
are not negotiable, and the things the first release deliberately does not
mitigate. `workstreams/14-security.md` is the work that verifies this doc.

## Assets

- A tenant's project files, Docker images and shell history on their volume.
- A tenant's tool logins inside the guest (gh, Codex, opencode) and their
  Claude session.
- A tenant's named secrets (API keys they chose to store).
- The platform CAs (user and host), hostd client certificates, the Key
  Vault wrapping key, Stripe and Logto credentials.
- The host itself: root on a host is every guest on it.
- Billing integrity: usage rows and the Stripe customer mapping.

## Actors

- **Another tenant**: has a guest on the same host, a valid certificate for
  their own project, and arbitrary code execution inside their guest. This
  is the primary adversary; agents run untrusted code by design.
- **An anonymous internet user**: can reach the gateway on 22, the api and
  dashboard on 443, and Logto.
- **A malicious or compromised agent** inside a tenant's own guest: can do
  anything the tenant can inside the guest, including using their tool
  logins. This is the tenant's risk, bounded by what the guest can reach.
- **The operator**: root on hosts. Trusted, audited, and (until LUKS) able
  to read volumes.
- **A stolen laptop**: holds a refresh token and an SSH certificate.

## Boundaries

From `ARCHITECTURE.md`, with the mechanism and the actor it stops:

1. **KVM between guest and host.** Cloud Hypervisor on KVM; the guest sees
   one block device, one tap, one vsock, one virtio-fs mount (read-only,
   exported by an unprivileged `virtiofsd` in a chroot), a serial console.
   Stops: a tenant or their agent reaching the host or the store's write
   path.
2. **nftables between guests.** Per-guest tap, default-drop forward chain,
   drop to the host, drop to IMDS, drop to all other guest ranges, NAT out.
   Stops: a tenant reaching another tenant's sshd, guestd, or dev servers.
3. **SSH certificates with project principals**, checked twice: at the
   gateway (route lookup plus principal match) and at the guest's sshd.
   12-hour validity, revocation list. Stops: a tenant or a stolen
   certificate opening another project; a stolen laptop after 12 hours or
   after `repose logout` from another device.
4. **mTLS per host** with the host id as CN; every command checked against
   the stream's identity. Stops: a compromised host acting on another
   host's guests.
5. **JWT scoping**: every api query is scoped by the `sub` in the token;
   cross-user lookups return 404. Stops: enumeration.
6. **Envelope encryption for named secrets**: Postgres holds ciphertext and
   a wrapped DEK; Key Vault holds the wrapping key; the api can wrap and
   unwrap but not export. Stops: a Postgres dump (including the R2 backups)
   revealing secrets.
7. **Resource limits per guest**: `MemoryMax`, `CPUQuota`, egress shaping,
   thin-volume size, build time and closure caps. Stops: a tenant degrading
   neighbours.
8. **Restricted Nix evaluation** of user fragments: pure, `restrict-eval`,
   no import-from-derivation, sandboxed builds, capped. Stops: a fragment
   reading host files or running unsandboxed code during a build.

## Non-negotiables

Rules that hold regardless of convenience. Each names its failure.

- **No inbound to hosts.** A host with a public port is a host whose
  hypervisor is reachable from the internet.
- **IMDS is blocked from guests.** The Azure metadata service issues
  managed-identity tokens; a guest that can reach it can act as the host.
- **Guests never share a bridge without the drop rules.** A shortcut that
  puts two taps on a plain bridge is a shared L2 between tenants.
- **Claude credentials are never copied or stored by the platform.**
  Anthropic's terms require users to authenticate with their own
  credentials; the copied file also does not refresh.
- **Secret values never leave `secrets.ciphertext` and the guest tmpfs.**
  Not in logs, not in `audit_log`, not in api responses, not in build logs.
- **Process samples carry names, CPU, memory, bytes. Nothing else.** The
  privacy policy says so in those words. guestd reads `/proc/<pid>/stat` and
  `/proc/<pid>/status` and nothing else of a process: it never opens
  `/proc/<pid>/cmdline` and never opens `/proc/<pid>/environ`, with the single
  exception below. `TestStraceNeverOpensCmdlineOrEnviron` in
  `internal/guestd` runs guestd under `strace -e openat` while it serves a
  Sample and asserts it, because a library added later that reads a command
  line would keep every other test green.
- **The one environment guestd reads is `$TMUX_PANE` of a hook's caller.** An
  agent wrapper that POSTs to `/run/repose/hooks.sock` without naming its tmux
  window is resolved by taking the peer's pid from `SO_PEERCRED` and reading
  that one variable out of its environment. Nothing else from that environment
  is read, kept, logged or sent. The alternative, sending the window name in
  the payload, is what every wrapper does when it can; this is the fallback
  so that a hook still reaches the user rather than being dropped.
  `TestStraceReadsEnvironOnlyForTheHookPaneLookup` asserts that this is the
  only environ open, and only on that path.
- **Every `Exec` is audited.** Operator convenience that skips the audit is
  an unrecorded access to tenant data.
- **Certificates expire in 12 hours** and the gateway checks revocation.
  A longer lifetime makes a stolen laptop a longer problem.
- **`security_type = Standard` and Intel hosts.** Not security in itself,
  but a Trusted Launch host silently has no `/dev/kvm`, and a fallback to
  containers "just for now" would collapse boundary 1.

## The abuse watch list

`internal/guestd/sample/watch.go` holds process names that are always reported
in a guest sample even when they fall below the top fifty by CPU, so a miner
that throttles itself to stay off the top of the list still appears in the
Grafana "Abuse" panel. It is a list of names worth seeing, not an accusation,
and it changes nothing about what is collected: only which of the names already
collected survive the trim.

```
xmrig  minerd  cpuminer  ccminer  cgminer  bfgminer  ethminer  nbminer
phoenixminer  t-rex  lolminer  xmr-stak  kdevtmpfsi  kinsing  tsm  sysrv
masscan  zmap  hashcat  john
```

Keep this list and `watch.go` in step; `TestWatchListIsNotEmpty` checks the
file is populated, and a reviewer checks the two agree.

## Not mitigated in the first release

Written down so nobody believes otherwise.

- **Operator access to tenant volumes.** Root on a host can read any thin
  volume. Mitigation is per-project LUKS with keys held by the api
  (DECISIONS R3-10). Until then the audit log records operator logins and
  the privacy policy does not claim otherwise.
- **Side channels between guests** (cache timing, Spectre-class). Cloud
  Hypervisor and the kernel mitigations are enabled; no further isolation
  (core pinning, no SMT sharing) is done. A tenant extracting another
  tenant's secrets by side channel is judged unlikely at this scale and
  price; a paying customer who needs that gets a dedicated host, which the
  scheduler does not yet support.
- **Nested-virtualization escape.** The guest-to-host boundary is KVM
  running inside Hyper-V. A KVM escape lands in the Azure VM, not in
  Azure. This is the same posture as AKS Pod Sandboxing.
- **Abuse detection is manual.** Process samples and egress are recorded
  and dashboarded; a human decides to suspend. No automatic kill.
- **Denial of service against the gateway or api.** Rate limits exist per
  user; no upstream DDoS protection beyond what Azure gives a public IP.
- **Supply chain of the agent overlay.** Agents are repackaged from
  upstream binary releases with pinned hashes; there is no independent
  verification of upstream builds.
- **A tenant's agent misusing the tenant's own tool logins.** Inside the
  guest, gh and Codex tokens are readable by any process as `dev`. That is
  the same exposure as on the tenant's laptop.

## Reporting

Security reports go to the owner's email listed on `repose.herakraft.co`.
Incident handling is in `ops/RUNBOOK.md` under "Suspected cross-tenant
access".
