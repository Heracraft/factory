# infra

Every cloud resource repose needs, declared here and created by `tofu apply`,
so that a new host, a replacement edge or a rebuilt control plane is a
reviewed diff rather than a remembered sequence of portal clicks.

Azure holds compute and storage only. Everything else in the system is
provider-agnostic on purpose (`docs/DESIGN.md` §16), so a Hetzner host module
is an addition here rather than a rewrite of anything else. The workstream
document is `docs/workstreams/11-infra-opentofu.md`.

```
infra/
  Makefile            make plan ENV=prod, make apply ENV=prod, make check
  bootstrap/          resource group and state account. Local state. Applied once.
  azure/
    modules/
      environment/    composes everything below; prod/ and staging/ are thin
      network/        VNet, three subnets, NAT gateway, one NSG per subnet
      host/           one repose host: VM, data disk, NixOS, join token
      edge/           SSH gateway and WireGuard hub VM
      coolify/        control-plane VM (stays Ubuntu)
      storage/        snapshot container, lifecycle rule, host identity
      keyvault/       the key that wraps every user's DEK
      nixos_anywhere/ shared provisioner for the two NixOS machines
    prod/             state key azure.tfstate
    staging/          state key staging.tfstate
  dns/                Cloudflare records, called by the environment module
  r2/                 Postgres backup bucket, state key r2.tfstate
  policy/tfsec/       the two rules a reviewer should not have to remember
```

Never commit `*.tfstate` or `*.local.tfvars`. `.terraform.lock.hcl` **is**
committed, deliberately: it is what makes a plan in CI resolve the same
provider versions as a plan on a laptop.

## Before anything

`docs/ops/AZURE-SETUP.md` lists the one-time human steps: subscription and
quota, resource providers, the `repose-prod` resource group, the state storage
account, `az login`, a budget alert, an SSH key, the Cloudflare and Logto
setup. Steps 1 to 9 must be done before the first apply here.

Tools come from the dev shell. `nix develop ./nix` has `tofu`, `az` and
`nixos-anywhere`; `nix develop ./nix#infra` adds `tfsec` for `make policy`.

Two environment values are read from the environment rather than a file:

```bash
az login                                # OpenTofu's Azure credentials
export CLOUDFLARE_API_TOKEN=...         # only for DNS and R2
```

## Values: what is committed and what is not

`prod.tfvars` holds the values that belong in review: sizes, disk sizes,
account names, the host list. `prod.local.tfvars` (git-ignored; copy
`prod.local.tfvars.example`) holds the values that are per-operator or
secret: which addresses may reach operator SSH, the operator public keys, the
path to the private key, the Cloudflare zone id and the join tokens. The
Makefile passes both when the second exists.

## Bootstrap

Production's resource group and state storage account were created by hand on
2026-09-19 (`docs/ops/AZURE-SETUP.md` steps 4 and 5) and are **not** managed
by OpenTofu: an apply that could destroy the state container could destroy the
state describing it (`docs/DECISIONS.md` I-18). `infra/bootstrap` describes
that shape anyway, so a second environment is one command:

```bash
make bootstrap ENV=staging
```

which creates `repose-staging` and keeps its state in production's account
under the key `staging.tfstate`. The `tofu import` commands for adopting
production's existing resources are at the top of `bootstrap/main.tf`; running
them is a deliberate decision, not a step in this list.

State locking is a blob lease. Versioning is on, so a corrupted state file is
restored from the previous blob version.

## Order of the first apply

The pieces depend on each other in one direction only:

1. **Edge first.** Hosts have no public IP, so every provisioner reaches them
   through the edge, and the module graph enforces that.
2. **Control plane.** Independent of hosts; its WireGuard peer needs the
   edge's public key, which does not exist until the edge is installed, so
   the first apply leaves its tunnel down (below).
3. **Hosts**, one at a time, each with a join token.

```bash
make plan  ENV=prod        # read it
make apply ENV=prod
```

The plan for an environment with `hosts = []` creates the network, the NAT
gateway, the snapshot account, the Key Vault, the edge and the control plane,
and nothing else.

## How the installer reaches a host

Through the edge, always. `nixos-anywhere`, the post-install `/dev/kvm` and
nested-virtualization checks, and the join-token delivery all connect to the
host's **private** address with the edge as an SSH jump host, and the module
graph makes every host depend on the edge having been installed first.

Hosts never get a public IP, not even a temporary one for the install
(`docs/DECISIONS.md` I-24). A public IP would need an inbound rule on the
hosts subnet, which is the one thing `policy/tfsec` forbids; the install
window is about ten minutes, not seconds; and what would be sitting in it is a
stock Ubuntu image accepting root SSH.

```
you (operator_cidrs) ──ssh──▶ edge :<edge_operator_ssh_port> ──ssh──▶ host :22
```

Two things have to be true for that to work, and both are worth checking
before the first apply:

- **Your address is in `operator_cidrs`.** The edge NSG opens the operator
  SSH port to that list and to the VNet, nothing else.
- **The edge's sshd is on `edge_operator_ssh_port`.** It defaults to 2222,
  because 22 belongs to the user-facing SSH gateway once workstream 06 lands.
  Until then, `nix/edge` serves sshd on 22 and there is no gateway competing
  for it, so **the first apply sets `edge_operator_ssh_port = 22`** in the
  local tfvars and moves it to 2222 in the same change that gives the edge its
  gateway.

The host side needs nothing configured: Azure's built-in `AllowVnetInBound`
rule is what lets the edge reach a host's sshd, which is why the hosts NSG can
have no rules of its own at all.

The provisioner shape this rests on — bastion connection, `install -d` then a
`file` provisioner then `remote-exec` — was exercised against a real sshd on
2026-09-19: the token arrived at mode 0600 and its contents appear nowhere in
the apply log, because it travels as file content and never as an argument.

## Adding a host

```bash
# M1, before the api exists: the one-host dev driver mints it (DECISIONS I-17)
hostdev init --host host-02                              # prints a token
# once the api is up:
# repose-admin hosts add --name host-02 --provider azure
```

Then:

1. Add `"host-02"` to `hosts` in `azure/prod/prod.tfvars`, in a commit
   somebody reads.
2. Add `host-02 = "<token>"` to `join_tokens` in `prod.local.tfvars`.
3. `make plan ENV=prod`, read it, `make apply ENV=prod`.

The apply creates the VM and its data disk, installs NixOS with
nixos-anywhere through the edge, checks that `/dev/kvm` exists and that
`kvm_intel` nested is `Y`, writes the token to `/run/repose/join-token` and
restarts `hostd`. `repose-admin hosts list` shows `ready` within about two
minutes of the reboot; if it does not, the runbook entry is
"Host never registered".

Leave the spent token in `prod.local.tfvars`. It is single-use and expires in
24 hours, so a stale entry is inert, and leaving it keeps later plans from
proposing to remove the delivery step. Replacing its value is how a token is
rotated.

The whole thing takes roughly ten minutes, most of it the kexec and the
closure copy.

## Rotating a join token

A host that never registered, or one whose token was seen by someone:

```bash
hostdev init --host host-02 --reissue                 # new token (M1)
# repose-admin hosts add --name host-02 --reissue      # once the api exists
# replace the value in prod.local.tfvars
make apply ENV=prod
```

Only the token-delivery step re-runs; the VM is not touched. By hand, the same
thing is the runbook's "Host never registered": write the token to
`/run/repose/join-token` over the edge jump and `systemctl restart hostd`.

The token never travels as a command-line argument, so it is not in the apply
log, and it lands on a tmpfs, so it is not on the disk
(`docs/DECISIONS.md` I-19).

## Wiring the control plane to the edge

The control-plane VM generates its own WireGuard key at first boot and never
hands the private half to anybody, so the peering is two manual moves:

```bash
ssh root@<control ip> cat /etc/wireguard/publickey     # add this as a peer on the edge
ssh -p 2222 root@<edge ip> cat /etc/wireguard/publickey
# put that value in prod.local.tfvars as edge_wireguard_public_key
make apply ENV=prod
ssh root@<control ip> /usr/local/sbin/repose-wg-setup  # brings wg0 up
```

## Draining and destroying a host

Shrinking the `hosts` list destroys a VM and everything on it. Do this, in
order:

1. `repose-admin hosts drain host-02` — the scheduler places nothing new.
2. Move or stop every project on it: `repose-admin projects move` for each,
   or let tenants stop them. `repose-admin hosts list --json` shows the count.
3. `repose-admin hosts retire host-02`.
4. Remove the name from `hosts` in `prod.tfvars`.
5. The data disk and its attachment carry `prevent_destroy`, so the plan will
   fail. That is the point. Delete the `prevent_destroy = true` lines in
   `azure/modules/host/main.tf` in a commit somebody reads, apply, and put
   them back.

Step 5 is friction on purpose: a host's data disk holds every tenant volume on
that host, and the mistake it prevents is a one-character edit to a list.

## Recovering from a stuck state lease

An apply that was killed leaves a lease on the state blob and the next one
says `Error acquiring the state lock` with an ID.

```bash
# Confirm nobody is running an apply anywhere. Then:
make force-unlock ENV=prod LOCK_ID=<the id from the message>
```

If the state file itself is wrong, the container has blob versioning:
`az storage blob list --account-name reposetfstate3912 --container-name
tfstate --include v` shows the versions, and
`az storage blob copy start` from a version restores it.

## Checks

```bash
make check                 # fmt-check + validate (all roots) + tfsec
make plan ENV=staging      # costs nothing; a plan creates nothing
```

`policy/tfsec/repose_tfchecks.yaml` holds two rules:

- **REPOSE-NET-001**: the NSG named `nsg-hosts` must have no security rules.
  Hosts have no inbound (`docs/DESIGN.md` §4, §7); Azure's built-in
  `AllowVnetInBound` already lets the edge's jump connection through, so an
  explicit rule here is always a mistake.
- **REPOSE-VM-001/002**: no VM may set `secure_boot_enabled` or
  `vtpm_enabled`. Trusted Launch disables nested virtualization, and a host
  with no `/dev/kvm` boots perfectly and serves nothing.

tfsec is in maintenance upstream; trivy is its successor. Two checks is a
small enough surface to port when that day comes.

## Cost

Re-checked on **2026-09-19** against the Azure Retail Prices API
(`https://prices.azure.com/api/retail/prices`, `armRegionName eq 'eastus'`,
`priceType eq 'Consumption'`), for what `prod.tfvars` actually creates: one
`D16s_v5` host with a 512 GB Premium SSD v2 data disk (DECISIONS I-14), the
edge, and **no control-plane VM** (`coolify_count = 0`, DECISIONS I-23). 730
hours to the month, Linux rates, no reservation.

| Resource | Unit price | Monthly |
|---|---|---|
| Host `Standard_D16s_v5` | $0.768/h | $560.64 |
| Host OS disk, Premium SSD P15 (256 GiB) + mount | $38.01 + $1.83 | $39.84 |
| Host data disk, Premium SSD v2, 512 GiB | $0.00011/GiB/h | $41.12 |
| — its 16,000 IOPS (3,000 free) | $0.000007/IOPS/h | $66.43 |
| — its 600 MB/s (125 free) | $0.000055/MBps/h | $19.07 |
| NAT gateway | $0.045/h + $0.045/GB processed | $32.85 + egress |
| Edge `Standard_D2s_v5` | $0.096/h | $70.08 |
| Edge OS disk, Premium SSD P6 (64 GiB) + mount | $10.21 + $0.47 | $10.68 |
| Two Standard static IPv4 (NAT, edge) | $0.005/h each | $7.30 |
| Blob, 500 GB of snapshots, Cool LRS | $0.0152/GB/month | $7.60 |
| Key Vault standard, operations | $0.03 per 10K | under $1 |
| R2, 50 GB | $0.015/GB/month | $0.75 |
| **Total, one host, no control plane, before egress** | | **about $857** |

Adding the control-plane VM in wave 3 — `coolify_count = 1`, a `D4s_v5` at
$140.16, its 256 GB OS disk at $39.84 and its static IP at $3.65 — takes it to
about **$1,040**, which is over the $1,000 monthly budget alert in
`docs/ops/AZURE-SETUP.md` step 7. Raise the budget to $1,200 at that point, or
drop the control plane to a `D2s_v5` and save $70.

The other thing this table says that the earlier estimate did not:
**Premium SSD v2 provisioned performance is most of the disk bill.** Capacity
is $41; the IOPS and throughput above the free tier are another $85.
`host_data_disk_iops` and `host_data_disk_mbps` are variables for exactly that
reason, and both can be raised in place later without downtime, so the first
host can start at the free tier (3,000 IOPS, 125 MB/s) and save $85 a month
until there is a guest whose builds justify more.

At launch sizes (`Standard_D64s_v5`, 2 TB data disk, control plane on) the
same table totals about **$2,850**. The $10k credit funds roughly three and a
half months of that with one host. Reservations generally cannot be bought
with sponsorship credits; check before assuming a reserved rate applies.

One figure is not from the API: Azure does not expose a NAT Gateway meter
under any `serviceName` in `eastus` that the retail endpoint will return, so
$0.045 per hour and $0.045 per GB processed are the published rates, not
queried ones.

## What is deliberately not here

- **The NixOS configurations.** `nix/hosts` and `nix/edge` are workstreams 01
  and 06; this only invokes them.
- **Coolify's application definitions.** Coolify keeps them in its own
  database; the click path is in `docs/ops/RUNBOOK.md` "Control plane".
- **The api's Entra app registration and client certificate**, and **the R2
  API token.** Both are credentials. Creating them is a human step next to the
  other identity setup in `docs/ops/AZURE-SETUP.md`, and neither belongs in a
  state file (`docs/DECISIONS.md` I-20). Pass the app's object id as
  `api_identity_object_id` and the Key Vault wrap/unwrap policy appears.
- **A Hetzner module.** Designed for in `docs/workstreams/11-infra-opentofu.md`
  §5, not built. The `host` module's variable surface (`name`, `join_token`,
  `class`, `data_disk_gb`) and its outputs (`private_ip`, `ssh_jump`) are the
  contract a second provider implements.
- **Automatic capacity.** Capacity is added by a human at an 80 percent memory
  alert (`docs/DECISIONS.md` R3-5).

## Notes for the workstreams this hands off to

- **01 host-nixos.** `nix/hosts/disko.nix` formats `/dev/sda`. On an Azure
  Gen2 VM the OS disk is normally `/dev/sda`, but the device name is not
  guaranteed across reboots or SKUs and `nixos-anywhere` will destroy whatever
  is at that path. `/dev/disk/azure/root` is the stable name. The data disk
  arrives at LUN 10, `/dev/disk/azure/scsi1/lun10`, and the host module's
  post-install check asserts it is there.
- **01 host-nixos.** `hostd.service` is the unit name this configuration
  restarts after writing a join token, taken from
  `docs/interfaces/host-conventions.md`. If the unit is named differently,
  set `hostd_unit` rather than renaming anything.
- **06 gateway-edge.** The edge subnet NSG opens 22, 443 and 51820/udp to the
  internet and the operator SSH port to `operator_cidrs` and the VNet, which
  is exactly the port list in `docs/workstreams/06-gateway-edge.md` §5.1. The
  operator sshd must be on 2222, because every host provisioner jumps through
  it while 22 belongs to the user gateway.
