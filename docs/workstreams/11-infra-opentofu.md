# Workstream 11: infrastructure (OpenTofu)

> **Host size is a variable (DECISIONS I-14).** `host_size` defaults to
> `Standard_D16s_v5` and `host_data_disk_gb` to 512 for the pre-launch month;
> launch values are `Standard_D64s_v5` and 2048. Validation rejects any size
> whose name is not `Standard_D[0-9]+s_v[56]` so an AMD or ARM size cannot be
> applied by mistake. The Coolify VM is `Standard_D4s_v5`, the edge
> `Standard_D2s_v5`.

## 1. Goal

Every cloud resource the platform needs is declared in `infra/` and created
by `tofu apply`, so a new host, a replacement edge, or a rebuilt control-plane
VM is a reviewed diff rather than a remembered sequence of portal clicks. The
Azure-specific surface is kept to compute and storage so that a second
provider module for hosts is an addition, not a rewrite.

## 2. Scope: builds

- `infra/azure/`: one root module per environment (`prod/`, `staging/`)
  composing these modules:
  - `network`: resource group, VNet `10.200.0.0/16`, subnet `hosts`
    (`10.200.1.0/24`) with a NAT gateway and an NSG allowing **no inbound**,
    subnet `edge` (`10.200.2.0/24`) allowing inbound 22/tcp and 51820/udp,
    subnet `control` (`10.200.3.0/24`) allowing inbound 80 and 443.
  - `host`: one `Standard_D64s_v5` per instance, `security_type =
    "Standard"` (Trusted Launch disables nested virtualization), Ubuntu
    24.04 marketplace image as the nixos-anywhere target, no public IP, one
    Premium SSD v2 data disk (2 TB, 16k IOPS, 600 MB/s to start), cloud-init
    that writes `/run/factory/join-token` from a variable and opens root SSH
    for the provisioner, a `null_resource` provisioner that runs
    `nixos-anywhere --flake .#host-<name> root@<private ip>` through the edge
    as a jump host, then a second provisioner that removes the temporary
    root key. Tag `factory:role=host`.
  - `edge`: one `Standard_B2s`, static public IP, DNS A record
    `ssh.factory.herakraft.co`, Ubuntu image, nixos-anywhere provisioner
    with `.#edge`. Root SSH after install is by operator certificate only.
  - `coolify`: one `Standard_D4s_v5`, Ubuntu 24.04 LTS, static public IP,
    DNS A records `factory.herakraft.co`, `api.factory.herakraft.co`,
    `auth.factory.herakraft.co`, 256 GB Premium SSD OS disk, cloud-init that
    installs Docker and runs Coolify's installer, then installs a WireGuard
    peer config so Prometheus and the api can reach the edge network. Its
    NSG allows 80, 443 and 22 from the owner's IP list only.
  - `storage`: storage account (LRS, hot), container `factory-snapshots`
    with a lifecycle rule moving blobs older than 7 days to cool and
    deleting blobs older than 45 days (the 30-day post-destroy window plus
    slack; the api deletes on schedule and this is the backstop). A
    user-assigned managed identity for hosts with `Storage Blob Data
    Contributor` scoped to that container.
  - `keyvault`: Key Vault with purge protection, one RSA-3072 key
    `factory-dek-wrap` with rotation policy 12 months, and an access policy
    for the api's identity (wrap/unwrap only, never get). The api on the
    Coolify VM authenticates with a client certificate stored in Coolify's
    secret store, since the Coolify VM is not an Azure identity target for
    containers.
- `infra/r2/`: Cloudflare provider, one bucket `factory-pg-backups` with a
  lifecycle rule deleting objects older than 35 days, and an API token
  scoped to that bucket for Coolify's backup job.
- `infra/dns/`: Cloudflare zone records for `herakraft.co` subdomains used
  above, so DNS is in the same apply as the addresses it points at.
- State backend: an Azure Storage container `tfstate` in a separate resource
  group created once by `infra/bootstrap/` (a tiny root module applied with
  local state and then never touched). Locking via blob leases.
- `infra/README.md`: how to bootstrap, how to add a host, how to rotate the
  join token, how to destroy a host safely (drain first).
- A `Makefile` target per root: `make plan ENV=prod`, `make apply ENV=prod`.

## 3. Scope: does not build

- The NixOS configurations themselves (`nix/hosts`, `nix/edge`): workstream
  01 and 06. This workstream only invokes them.
- Coolify application definitions (api, dashboard, Logto): workstream 05 and
  08 document theirs; Coolify holds them in its own database, and a
  `docs/ops/coolify.md` note records the click path until Coolify's API is
  used.
- Hetzner module: designed for in §5, not built.
- Automatic capacity (api-driven `tofu apply`): later, per R3-5.

## 4. Interfaces

Owns: variable names and outputs of each module, the join-token handoff
(`/run/factory/join-token`), the tag scheme, DNS names.

Consumes: `interfaces/host-conventions.md` (what the host expects at boot),
`interfaces/grpc-hostd.md` (Register uses the join token).

## 5. Design detail

### Adding a host

```
factory-admin hosts add --name host-03 --provider azure   # mints join token, prints it
tofu -chdir=infra/azure/prod apply -var 'hosts=["host-01","host-02","host-03"]' \
     -var 'join_tokens={"host-03":"<token>"}'
```

The apply creates the VM, attaches the disk, runs nixos-anywhere with the
disko layout from `nix/hosts/disko.nix` (root on the OS disk, `vg-guests`
on the data disk), reboots into NixOS, and hostd registers on first boot.
The token is single-use and expires in 24 hours. `factory-admin hosts list`
shows the host as `ready` within two minutes of the reboot or the runbook
entry "Host never registered" applies.

### Why Ubuntu as the install target

Azure has no NixOS marketplace image and building one adds an image
pipeline. nixos-anywhere replaces the running Ubuntu with NixOS over SSH
using kexec, which takes about four minutes and needs nothing but a root
login. The Ubuntu image is a delivery vehicle; nothing from it survives.

### Provider portability

The `host` module has a provider-neutral variable surface: `name`,
`join_token`, `class` (`azure-d64s-v5` today), `data_disk_gb`. Its outputs
are `private_ip` and `ssh_jump`. A `hetzner_host` module with the same
variables and outputs, using the Hetzner Robot provider for an AX162-R and
the same nixos-anywhere provisioner, slots in with one change to the root
module's `for_each`. The NixOS host config already takes its guest CIDR
and WireGuard keys from `Register`, so it does not care which provider it
runs on. Blob as the snapshot target is behind `hostd`'s
`SnapshotStore` interface (Azure Blob and S3-compatible implementations),
so a Hetzner host uses Hetzner Object Storage or R2.

### Cost table (East US, on-demand, September 2026)

| Resource | Monthly |
|---|---|
| Host `D64s_v5` | ~$2,240 (1-year reserved ~$1,380) |
| Host data disk, Premium SSD v2, 2 TB, 16k IOPS | ~$200 |
| NAT gateway plus egress | ~$35 plus $0.045 per GB egress beyond the first 100 GB |
| Edge `B2s` plus static IP | ~$35 |
| Coolify VM `D4s_v5` plus 256 GB disk plus IP | ~$180 |
| Blob, 1 TB snapshots cool tier | ~$15 |
| Key Vault | under $5 |
| R2, 50 GB | under $1 |
| Total with one host | ~$2,700 |

The $10k credit funds roughly three and a half months with one host and
no reservation. Reservations cannot be bought with credits on most Azure
sponsorship SKUs; check before assuming the reserved column applies.

## 6. Failure modes

| Failure | Outcome |
|---|---|
| nixos-anywhere fails mid-install | the VM is left on Ubuntu or half-kexec'd; `tofu apply` reports the provisioner error; re-running the apply re-taints the host and retries from the image (the data disk is preserved because it is a separate resource and `prevent_destroy = true`). |
| Join token expired before first boot | hostd logs `register_fail reason=token_expired` every 30 s; runbook "Host never registered": mint a new token, write it to `/run/factory/join-token` over the edge jump, restart hostd. |
| `security_type` left at the default | the VM boots but `/dev/kvm` is absent; hostd refuses to start with `kvm_missing`. The variable has no default so the plan fails without an explicit value. |
| Data disk detached or lost | hostd fails `pool_missing` at start and marks every guest on the host `error`; recovery is restore from Blob onto another host (runbook "Host loss"). `prevent_destroy` and a `lifecycle.ignore_changes` on the disk attachment guard the common mistakes. |
| State lock stuck | `tofu force-unlock` after confirming no apply is running; documented in `infra/README.md`. |
| DNS record points at a replaced IP | static IPs are separate resources with `prevent_destroy`, so a VM replacement keeps its address. |

## 7. Testing

- `tofu validate` and `tofu plan` in CI against a `staging` root with
  `hosts=[]` (no cost) on every change to `infra/`.
- A monthly exercise on staging: create one host, let it register, destroy
  it. Time and any manual steps recorded in `infra/README.md`.
- `checkov` or `tfsec` in CI for the NSG rules: any inbound rule on the
  hosts subnet fails the build.
- Manual once: confirm `/dev/kvm` exists and `cat /sys/module/kvm_intel/parameters/nested` is `Y` on a freshly applied host.

## 8. Rollback

`tofu apply` with the previous variable values. Hosts are never destroyed
by a rollback unless the host list shrinks, and shrinking requires the host
to be `retired` in the api first (a `precondition` reads
`factory-admin hosts list --json`). State is versioned in the storage
container with soft delete enabled, so a corrupted state file is restored
from the previous version.

## 9. Checklist

- [ ] `infra/bootstrap` applied once; state container exists with soft
      delete and versioning. Evidence: `az storage container show` output.
- [ ] `tofu plan` on `prod` with the current host list is empty. Evidence:
      plan output pasted.
- [ ] A host created by `tofu apply` registers with the api without manual
      steps. Evidence: `factory-admin hosts list` showing `ready`, and the
      apply log.
- [ ] `/dev/kvm` present and nested enabled on a fresh host. Evidence:
      command output.
- [ ] NSG on the hosts subnet has no inbound rules; a `tfsec` rule enforces
      it. Evidence: CI run.
- [ ] IMDS is reachable from the host (hostd needs nothing from it, but the
      Azure agent does) and blocked from guests (workstream 01 verifies the
      nftables rule; this item checks the NSG does not interfere). Evidence:
      curl from host succeeds.
- [ ] Data disk has `prevent_destroy`; a plan that would replace it fails.
      Evidence: a deliberate attempt, output pasted.
- [ ] Blob lifecycle rule exists and a test blob aged past 45 days is
      deleted (use a backdated test in staging). Evidence: rule JSON and the
      observed deletion.
- [ ] Key Vault key exists, the api identity can wrap and unwrap and cannot
      `get`. Evidence: `az keyvault key` calls, including the denied one.
- [ ] R2 bucket, token and lifecycle rule exist; Coolify's backup job
      succeeded once and a restore was rehearsed. Evidence: Coolify backup
      log and restore timing.
- [ ] Edge and Coolify DNS names resolve to their static IPs. Evidence:
      `dig` output.
- [ ] `infra/README.md` covers bootstrap, add host, rotate token, drain and
      destroy host, force-unlock, and someone followed it end to end.
      Evidence: their notes merged into it.
- [ ] Cost table re-checked against the Azure price API on the date of
      the first apply. Evidence: date and figures in the README.
