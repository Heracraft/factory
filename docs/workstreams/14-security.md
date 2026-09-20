# Workstream 14: security

## 1. Goal

Verify, not assume, that one tenant cannot reach another tenant's guest,
data, or credentials, and that the platform's own privacy promises are
mechanically true. This workstream runs alongside every other one and ends
with the isolation tests in `../CHECKLIST.md` passing on a shared host and
the policy text published.

## 2. Scope: builds

- `../SECURITY.md`: the threat model, kept current as decisions land.
- `test/isolation/`: a Go test suite that runs against a real host with two
  guests owned by two users and asserts every boundary in §5.
- The privacy policy and terms of service text in `apps/web/src/content/
  legal/`, with the two required passages below verbatim.
- A review checklist for each other workstream, applied at their PR time,
  recorded as a comment in `STATUS.md`.
- `repose-admin audit` to query `audit_log`, and the audit coverage list in
  §5.
- An incident response note in `../ops/RUNBOOK.md`: who is told, what is
  captured, how a tenant is notified.

## 3. Scope: does not build

- The isolation mechanisms themselves: nftables and bridge layout
  (workstream 01), KVM and virtio-fs configuration (01, 03), sshd
  principals (02), certificate issuance (05), gateway checks (06), secrets
  encryption (05). This workstream tests them and blocks release on them.
- Per-project LUKS. Recorded as not mitigated (R3-10).
- A bug bounty or external audit. After launch.

## 4. Interfaces

Owns: `../SECURITY.md`, the isolation test suite, the legal text.

Consumes: every interface, read-only.

## 5. Design detail

### Boundaries and what proves each

| Boundary | Mechanism | Test in `test/isolation/` |
|---|---|---|
| Guest A cannot reach guest B | per-guest tap, nftables `guest_fwd` default drop, no shared L2 | from A: `ping`, `arping`, `nmap -p 22,5000` against B's IP all fail; `tcpdump` on B's tap sees nothing from A's MAC |
| Guest cannot reach host | `guest_in` drop (ICMP echo excepted at 5/s, DECISIONS I-18) | from A: connect to host `.1` on 22, 9101, 8080 fails; a `ping` flood of `.1` loses most packets |
| Guest cannot reach IMDS | explicit drop of `169.254.169.254/32` in `guest_fwd` | `curl -H Metadata:true http://169.254.169.254/...` times out from A |
| Guest cannot reach other hosts' guest ranges | drop `10.64.0.0/12` | connect to another host's guest IP fails |
| Guest cannot write the store | virtio-fs exported read-only, `virtiofsd --sandbox namespace` as an unprivileged user (I-48) | `touch /nix/store/x` fails; `ls /nix/store/.links` is absent or unreadable |
| A guest escape does not land as root | `guest@<id>` runs Cloud Hypervisor as `hostd` with `DevicePolicy=closed`, no capabilities, a private guests directory (I-51) | on the host: `ps -o user= -p $(systemctl show -p MainPID --value guest@<id>)` is `hostd`; `systemctl show guest@<id> -p User,NoNewPrivileges,DevicePolicy` |
| Guest cannot see host block devices | only its own thin volume is a virtio-blk device | `lsblk` shows one disk |
| Guest cannot escape memory or CPU limits | `MemoryMax` and `CPUQuota` on the transient unit, and CH's own limits | a fork bomb and a memory hog in A leave B's benchmark within 10 percent |
| A's certificate cannot open B | gateway principal check, guest sshd `AuthorizedPrincipalsFile` | `ssh b.user@ssh...` with A's cert is rejected at the gateway with `certificate not valid for this project`; a direct `ssh` to B's IP over the operator WireGuard with A's cert is rejected by sshd |
| An expired or revoked certificate is rejected | validity window, revocation list | issue, revoke, connect: rejected within 30 s of revocation |
| A user cannot read another user's project via the api | every query scoped by `user_id` from the JWT | `GET /projects/<B's id>` as A is 404, not 403 (existence is not leaked) |
| A user cannot read secret values | api never returns them | `GET /secrets` has no `value` field; `audit_log` has no values |
| hostd for host X cannot act on host Y | mTLS CN is the host id and every command is checked against it | a replayed command for Y's guest on X's stream returns `forbidden` |
| Hook socket cannot be used by one agent to spoof another project | the socket is inside the guest, and guestd stamps the guest id; hostd stamps the project id from its own table | a forged payload with another project id is ignored and logged `hook_bad_payload` |
| Console logs do not carry terminal contents | the guest's serial console shows only kernel and systemd output; user shells are on ptys | grep a session's typed text in Loki: absent |

### Policy text requirements

The privacy policy must contain, verbatim:

> We sample the processes running in your environment once a minute and
> record their names, CPU time, memory use and network bytes. We never
> record command-line arguments, environment variables, file paths, file
> contents, terminal contents, or the prompts you give to any agent.

The terms must contain, in substance:

> Coding agents such as Claude Code run inside your environment under your
> own account with that agent's provider. repose does not hold, proxy, or
> resell those credentials. You are responsible for complying with each
> provider's terms for hosted use.

The reason these are here rather than in a legal doc alone: the first is a
promise the `obs` redaction and the `Sample` message shape must keep, and
the second is the condition under which Anthropic permits Claude Code on a
hosted platform (each user authenticates with their own credentials).

### Secrets handling review

Applied to workstreams 05, 07 and 04 at review:

- Named secrets: ciphertext only in Postgres; DEK wrapped by Key Vault;
  the api's Key Vault permission is wrap/unwrap only; plaintext exists in
  api memory for the duration of a request and in the guest's tmpfs.
- Tool logins: copied laptop to guest over the user's own SSH session; the
  api does not see them; the guest file modes are 0600 `dev`.
- Claude credentials: never copied; `rg 'credentials.json' cmd internal`
  returns only the explicit exclusion in the CLI's sync list.
- CA private keys: in the api's secret store (Coolify secret injected as
  an env var pointing at a mounted file), never in Postgres, never logged.
- Join tokens and hostd certificates: single-use and rotated monthly.

### Audit coverage

`audit_log` rows for: every certificate issue and revoke, every `Exec` over
gRPC or vsock, every `repose-admin` command, every operator SSH login to a
host or the edge, every secret set or delete (name only), every user
suspension, every restore. `repose-admin audit --user`, `--project`,
`--since` query it. Retention: indefinite.

### Operator access

Operators reach hosts and the edge only over the edge's WireGuard with a
certificate from the Host CA (`repose-admin operator-cert`), 8-hour
validity. There are no operator passwords. Touching a guest goes through
`repose-admin exec`, which is audited; `virsh`-style direct access to a
guest's console is available only from the host and is logged by the PAM
hook.

### Incident basics

If a tenant boundary is found broken: stop scheduling (`repose-admin hosts
drain --all`), snapshot the affected guests, capture hostd and gateway logs
for the window, notify affected users within 72 hours with what was
exposed, record the timeline in `docs/incidents/YYYY-MM-DD.md`. The runbook
entry "Suspected cross-tenant access" has the commands.

## 6. Failure modes

| Failure | Outcome |
|---|---|
| An isolation test fails in CI | the release is blocked; the owning workstream's checklist item reopens. |
| The policy text and the `Sample` message drift (a new field) | the `obs` naming test includes a fixture of the policy sentence and fails if `ProcSample` gains a field not listed in `SECURITY.md`. |
| An operator logs in without a certificate | impossible by sshd config; a password login attempt is logged and alerts as `GatewayAuthSpike` if repeated. |

## 7. Testing

`test/isolation/` needs a real host with two guests. It runs in a nightly
job against the staging host and before every release. Each test is named
after the boundary row above. The api-level tests (`404 not 403`, no secret
values) run in ordinary CI against the fake hostd.

## 8. Rollback

Not applicable; this workstream adds tests and text. If a test is found to
be wrong, fix the test with a `DECISIONS.md` note explaining what the
boundary actually is.

## 9. Checklist

- [ ] `../SECURITY.md` matches the current design: every boundary in
      `ARCHITECTURE.md` has a row here. Evidence: a diff review by someone
      other than the author.
- [ ] Every row in the boundary table has a passing test on a shared host
      with two guests from two users. Evidence: test output pasted with
      host id and date.
- [ ] The fork bomb and memory hog test leaves the neighbour within 10
      percent. Evidence: numbers.
- [ ] Privacy policy and terms contain the two required passages. Evidence:
      the page URL and a grep of the source.
- [ ] `rg 'credentials.json'` shows only the exclusion. Evidence: output.
- [ ] Every audited action in §5 writes an `audit_log` row. Evidence: one
      row per action type, triggered deliberately.
- [ ] Operator access works only with a certificate; a password attempt is
      logged. Evidence: sshd log lines.
- [ ] Secrets review comments exist in `STATUS.md` for workstreams 04, 05,
      07. Evidence: the lines.
- [ ] Incident runbook entry exists and the commands in it were run once
      on staging. Evidence: notes.
