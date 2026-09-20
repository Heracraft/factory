# Workstream 11: infrastructure (OpenTofu)

> **Host size is a variable (DECISIONS I-14).** `host_size` defaults to
> `Standard_D16s_v5` and `host_data_disk_gb` to 512 for the pre-launch month;
> launch values are `Standard_D64s_v5` and 2048. Validation rejects any size
> whose name is not `Standard_D[0-9]+s_v[567]` so an AMD or ARM size cannot be
> applied by mistake. The Coolify VM is `Standard_D4s_v5`, the edge
> `Standard_D2s_v5`. Production actually runs the v7 generation of the same
> sizes because the subscription cannot deploy v5 or v6 in East US
> (DECISIONS I-40); v6 and v7 are NVMe-only, so `host_data_disk_device`
> and the named `host-01`/`edge-01` nix configurations carry the NVMe
> device names.

> **State backend (created 2026-09-19):** resource group `repose-prod`,
> storage account `reposetfstate3912`, container `tfstate`, key
> `azure.tfstate`, `use_azuread_auth = true`. Subscription
> `5f27aace-dd8c-4dc0-95bf-b59ee8de7d70`, tenant
> `3d6e97f4-af27-4cea-8bf3-f42718e67189`, region `eastus`.

## 1. Goal

Every cloud resource the platform needs is declared in `infra/` and created
by `tofu apply`, so a new host, a replacement edge, or a rebuilt control-plane
VM is a reviewed diff rather than a remembered sequence of portal clicks. The
Azure-specific surface is kept to compute and storage so that a second
provider module for hosts is an addition, not a rewrite.

## 2. Scope: builds

- `infra/azure/`: one root module per environment (`prod/`, `staging/`)
  composing these modules:
  - `network`: VNet `10.200.0.0/16` in the existing resource group (the
    group is created by `infra/bootstrap`, not here, DECISIONS I-19), subnet
    `hosts` (`10.200.1.0/24`) with a NAT gateway and an NSG carrying **no
    security rules at all** — Azure's built-in `AllowVnetInBound` is what
    lets the edge's jump connection through, so an explicit rule here is
    always a mistake — subnet `edge` (`10.200.2.0/24`) allowing inbound 22,
    443, 51820/udp and the operator SSH port, subnet `control`
    (`10.200.3.0/24`) allowing inbound 80, 443 and 22 from configured
    address lists.
  - `host`: one VM per instance at `host_size`, `security_type =
    "Standard"` (Trusted Launch disables nested virtualization), Ubuntu
    24.04 marketplace image as the nixos-anywhere target, no public IP, one
    Premium SSD v2 data disk at `host_data_disk_gb` (16k IOPS, 600 MB/s),
    cloud-init that opens root SSH for the provisioner and carries nothing
    secret, a provisioner that runs `nixos-anywhere --flake
    .#<host flake attr> root@<private ip>` through the edge as a jump host,
    a post-install check that `/dev/kvm` exists and `kvm_intel` nested is
    `Y`, and a final provisioner that writes the join token to
    `/run/repose/join-token` as file content and restarts `hostd`
    (DECISIONS I-20: cloud-init cannot deliver it, because nixos-anywhere
    replaces the system that ran cloud-init). Tag `repose:role=host`.
  - `edge`: one VM at `edge_size` (default `Standard_D2s_v5`, DECISIONS
    I-23), static public IP with `prevent_destroy`, DNS A record
    `ssh.repose.herakraft.co`, Ubuntu image, nixos-anywhere provisioner
    with `.#edge`. Its subnet NSG opens 22 (user gateway), 443 (preview
    stub) and 51820/udp (WireGuard hub) to the internet, and the operator
    sshd port 2222 to the operator address list and the VNet, which is the
    port list workstream 06 §5.1 owns. Operator SSH after install is by
    certificate only, on 2222, because 22 belongs to the gateway and every
    host provisioner jumps through it.
  - `coolify`: `coolify_count` of them, defaulting to **0** until wave 3
    (DECISIONS I-24); when 1, one `Standard_D4s_v5`, Ubuntu 24.04 LTS, static public IP,
    DNS A records `repose.herakraft.co`, `api.repose.herakraft.co`,
    `auth.repose.herakraft.co`, 256 GB Premium SSD OS disk, cloud-init that
    installs Docker and runs Coolify's installer, then installs a WireGuard
    peer config so Prometheus and the api can reach the edge network. Its
    NSG allows 80, 443 and 22 from the owner's IP list only.
  - `storage`: storage account (LRS, hot), container `repose-snapshots`
    with a lifecycle rule moving blobs older than 7 days to cool and
    deleting blobs older than 45 days (the 30-day post-destroy window plus
    slack; the api deletes on schedule and this is the backstop). A
    user-assigned managed identity for hosts with `Storage Blob Data
    Contributor` scoped to that container.
  - `keyvault`: Key Vault with purge protection, one RSA-3072 key
    `repose-dek-wrap` with rotation policy 12 months, and an access policy
    for the api's identity (wrap/unwrap only, never get) created when
    `api_identity_object_id` is supplied. The api on the Coolify VM
    authenticates with a client certificate stored in Coolify's secret
    store, since the Coolify VM is not an Azure identity target for
    containers; the app registration and that certificate are a human step
    (DECISIONS I-21).
- `infra/r2/`: Cloudflare provider, one bucket `repose-pg-backups` with a
  lifecycle rule deleting objects older than 35 days and aborting multipart
  uploads left incomplete for 7. The API token scoped to that bucket stays a
  human step (DECISIONS I-21): a token created here would sit in the state
  file in clear text for the life of the bucket.
- `infra/dns/`: Cloudflare zone records for `herakraft.co` subdomains used
  above, so DNS is in the same apply as the addresses it points at.
- State backend: the Azure Storage container `tfstate` in `repose-prod`,
  created by hand on 2026-09-19 and read but never managed by the
  environment roots (DECISIONS I-19). `infra/bootstrap/` is the tiny root
  module, applied with local state, that declares the same shape for a new
  environment; `make bootstrap ENV=staging` creates `repose-staging` and
  keeps its state in the same account under a different key. Locking via
  blob leases.
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
(`/run/repose/join-token`), the tag scheme, DNS names.

Consumes: `interfaces/host-conventions.md` (what the host expects at boot),
`interfaces/grpc-hostd.md` (Register uses the join token). For M1 the token
is minted by `hostdev init`, the one-host dev driver in workstream 03
(DECISIONS I-17), not by `repose-admin`; workstream 05 is not on the path
between this workstream and a registered host.

## 5. Design detail

### Adding a host

```
hostdev init --host host-03            # M1: mints the join token and prints it
# once the api exists: repose-admin hosts add --name host-03 --provider azure
# add "host-03" to hosts in azure/prod/prod.tfvars, in a reviewed commit
# add host-03 = "<token>" to join_tokens in prod.local.tfvars (git-ignored)
make -C infra plan ENV=prod && make -C infra apply ENV=prod
```

The token can also be passed inline as `-var 'join_tokens={"host-03":"..."}'`,
but then the next apply proposes to remove the delivery step. Leaving the
spent token in the local tfvars file is inert (single use, 24-hour expiry)
and keeps later plans quiet; replacing its value is how a token is rotated.

The apply creates the VM, attaches the disk, runs nixos-anywhere with the
disko layout from `nix/hosts/disko.nix` (root on the OS disk, `vg-guests`
on the data disk), reboots into NixOS, and hostd registers on first boot.
The token is single-use and expires in 24 hours. `repose-admin hosts list`
shows the host as `ready` within two minutes of the reboot or the runbook
entry "Host never registered" applies.

### How the installer reaches a host

Through the edge. `nixos-anywhere`, the post-install `/dev/kvm` and nested
checks, and the join-token delivery all connect to the host's private address
with the edge as an SSH jump host, and the module graph makes a host depend on
the edge being installed first. Hosts never get a public IP, not even a
temporary one during the install: that would need an inbound rule on the hosts
subnet, which is the one thing the tfsec policy forbids and §4 and §7 of
`DESIGN.md` promise never exists, and the window is about ten minutes of a
stock Ubuntu image accepting root SSH. DECISIONS I-25 has the full reasoning.

The edge's operator sshd must be listening on `edge_operator_ssh_port` for
that to work. It defaults to 2222 because 22 belongs to the user gateway;
until workstream 06 gives the edge that gateway, `nix/edge` serves sshd on 22
and the first apply sets `edge_operator_ssh_port = 22`.

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
| Edge `D2s_v5` plus static IP | ~$75 |
| Coolify VM `D4s_v5` plus 256 GB disk plus IP | ~$180 |
| Blob, 1 TB snapshots cool tier | ~$15 |
| Key Vault | under $5 |
| R2, 50 GB | under $1 |
| Total with one host | ~$2,700 |

`infra/README.md` carries the version of this table re-derived from the Azure
Retail Prices API on 2026-09-19, for what `prod.tfvars` actually creates:
about **$857** a month with one pre-launch host and no control-plane VM,
about **$1,040** once wave 3 turns that VM on, which is over the $1,000 budget
alert in `ops/AZURE-SETUP.md` step 7, and about **$2,850** at launch sizes.
Most of the difference from the estimate above is Premium SSD v2 provisioned
IOPS and throughput, which are variables and can be raised in place later. The $10k credit funds roughly three and a
half months at launch sizes with one host and no reservation. Reservations cannot be bought with credits on most Azure
sponsorship SKUs; check before assuming the reserved column applies.

## 6. Failure modes

| Failure | Outcome |
|---|---|
| nixos-anywhere fails mid-install | the VM is left on Ubuntu or half-kexec'd; `tofu apply` reports the provisioner error; re-running the apply re-taints the host and retries from the image (the data disk is preserved because it is a separate resource and `prevent_destroy = true`). |
| Join token expired before first boot | hostd logs `register_fail reason=token_expired` every 30 s; runbook "Host never registered": mint a new token, write it to `/run/repose/join-token` over the edge jump, restart hostd. |
| `security_type` left at the default | the VM boots but `/dev/kvm` is absent; hostd refuses to start with `kvm_missing`. The variable has no default so the plan fails without an explicit value. |
| Data disk detached or lost | hostd fails `pool_missing` at start and marks every guest on the host `error`; recovery is restore from Blob onto another host (runbook "Host loss"). `prevent_destroy` and a `lifecycle.ignore_changes` on the disk attachment guard the common mistakes. |
| State lock stuck | `tofu force-unlock` after confirming no apply is running; documented in `infra/README.md`. |
| DNS record points at a replaced IP | static IPs are separate resources with `prevent_destroy`, so a VM replacement keeps its address. |

## 7. Testing

- `tofu validate` and `tofu plan` in CI against a `staging` root with
  `hosts=[]` (no cost) on every change to `infra/`.
- A monthly exercise on staging: create one host, let it register, destroy
  it. Time and any manual steps recorded in `infra/README.md`.
- `tfsec` in CI for the NSG rules: `infra/policy/tfsec/repose_tfchecks.yaml`
  (tfsec only loads custom checks from files whose names end
  `_tfchecks.yaml`). REPOSE-NET-001 fails the build on any security rule on
  `nsg-hosts`; REPOSE-VM-001 and REPOSE-VM-002 fail it on any VM with Secure
  Boot or a vTPM, which is how a "hardened" host with no `/dev/kvm` gets
  caught. `make -C infra check` runs fmt, validate and these.
- Manual once: confirm `/dev/kvm` exists and `cat /sys/module/kvm_intel/parameters/nested` is `Y` on a freshly applied host.

## 8. Rollback

`tofu apply` with the previous variable values. Hosts are never destroyed
by a rollback unless the host list shrinks, and shrinking requires the host
to be `retired` in the api first (a `precondition` reads
`repose-admin hosts list --json`). State is versioned in the storage
container with soft delete enabled, so a corrupted state file is restored
from the previous version.

## 9. Checklist

- [ ] `infra/bootstrap` applied once; state container exists with soft
      delete and versioning. Evidence: `az storage container show` output.
- [ ] `tofu plan` on `prod` with the current host list is empty. Evidence:
      plan output pasted.
- [ ] A host created by `tofu apply` registers with the api without manual
      steps. Evidence: `repose-admin hosts list` showing `ready`, and the
      apply log.
- [ ] `/dev/kvm` present and nested enabled on a fresh host. Evidence:
      command output.
- [ ] NSG on the hosts subnet has no inbound rules; the `tfsec` rule
      REPOSE-NET-001 enforces it. Evidence: CI run, plus the rule firing on a
      fixture that adds one.
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

## 10. What is left for the owner

Nothing in `infra/` has been applied, and the owner's instruction is that the
apply happens from `main` after this branch and workstream 01's are merged, so
the host installs from the merged flake rather than this branch's snapshot of
the skeleton. Every checklist item above that says "a host", "a fresh host",
"a test blob" or "Coolify's backup job" waits on that, and `infra/README.md`
is the order to do it in.

Blob soft delete on `reposetfstate3912` was the one change made outside
OpenTofu: 30 days on blobs and on containers, versioning was already on.

Three things wait on a human rather than on an apply:

- **The api's Entra app registration**, its client certificate, and the R2
  API token (DECISIONS I-21). Pass the app's object id as
  `api_identity_object_id` and the Key Vault wrap/unwrap policy appears.
- **A Cloudflare API token** in `CLOUDFLARE_API_TOKEN` and the zone id in the
  local tfvars, before `manage_dns` can be true or `infra/r2` can be planned.
- **`edge_operator_ssh_port = 22`** in the local tfvars for the first apply,
  until workstream 06 moves the edge's operator sshd off the port the user
  gateway wants.
