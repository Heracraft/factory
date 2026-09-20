variable "subscription_id" {
  type        = string
  description = "Azure subscription holding every repose resource."
}

variable "resource_group_name" {
  type        = string
  description = "Existing resource group, created by `make bootstrap ENV=staging`."
  default     = "repose-staging"
}

variable "zone" {
  type        = string
  description = "Availability zone for hosts and their data disks."
  default     = "1"
}

variable "operator_cidrs" {
  type        = list(string)
  description = "Addresses allowed to reach operator SSH."
}

variable "control_web_cidrs" {
  type        = list(string)
  description = "Addresses allowed to reach 80 and 443 on the control plane."
}

variable "operator_authorized_keys" {
  type        = list(string)
  description = "Operator SSH public keys."
}

variable "ssh_private_key_path" {
  type        = string
  description = "Path to the matching private key on the machine running tofu."
}

variable "api_identity_object_id" {
  type        = string
  description = "Entra object id of the api's service principal, or null until it exists."
  default     = null
}

variable "flake_path" {
  type        = string
  description = "Absolute path to the repository's nix/ directory."
}

variable "build_on" {
  type        = string
  description = "nixos-anywhere --build-on: auto, local or remote."
  default     = "auto"
}

variable "hosts" {
  type        = list(string)
  description = "Hosts that exist in staging. Kept empty except during the monthly create-register-destroy exercise."
  default     = []
}

variable "dns_prefix" {
  type        = string
  description = "Label under the DNS zone. Staging must not collide with prod's names."
  default     = "repose-staging"
}

variable "host_data_disk_device" {
  type        = string
  description = "Device the data disk appears at on an installed host: /dev/disk/azure/scsi1/lun10 on SCSI sizes (v5), /dev/nvme1n1 on NVMe-only sizes (v6, v7)."
  default     = "/dev/disk/azure/scsi1/lun10"
}

variable "edge_flake_attr" {
  type        = string
  description = "nixosConfigurations attribute installed on the edge; a named one (edge-01) carries what differs from the generic edge, such as the NVMe OS device of a v7 size."
  default     = "edge"
}

variable "host_flake_attrs" {
  type        = map(string)
  description = "Per-host nixosConfigurations attribute in nix/flake.nix (for example host-01 = \"host-01\"); a host absent here installs the generic `host`."
  default     = {}
}

variable "join_tokens" {
  type        = map(string)
  description = "host name to single-use registration token. Passed with -var on the apply that adds a host; never committed."
  sensitive   = true
  default     = {}
}

variable "host_size" {
  type        = string
  description = "Azure VM size for hosts."
  default     = "Standard_D16s_v5"
}

variable "host_class" {
  type        = string
  description = "Capacity class recorded on each host."
  default     = "azure-d16s-v5"
}

variable "host_security_type" {
  type        = string
  description = <<-EOT
    Azure security type for hosts. Deliberately has no default, so a plan that
    does not pass the environment's tfvars fails instead of quietly picking
    one; only "Standard" is accepted, because Trusted Launch and Confidential
    VMs disable nested virtualization (docs/DESIGN.md §4).
  EOT
}

variable "host_data_disk_gb" {
  type        = number
  description = "Premium SSD v2 data disk per host."
  default     = 512
}

variable "host_data_disk_iops" {
  type        = number
  description = "Provisioned IOPS on each host's data disk. The first 3,000 are free."
  default     = 16000
}

variable "host_data_disk_mbps" {
  type        = number
  description = "Provisioned throughput MB/s on each host's data disk. The first 125 are free; Azure caps this at a quarter of the IOPS."
  default     = 600
}

variable "edge_operator_ssh_port" {
  type        = number
  description = <<-EOT
    Port the edge's operator sshd listens on; every host provisioner jumps
    through it. 2222 is the target state, because 22 belongs to the
    user-facing SSH gateway. Until workstream 06 gives the edge that gateway,
    nix/edge serves sshd on 22 and this must be 22
    (infra/README.md, "How the installer reaches a host").
  EOT
  default     = 2222
}

variable "edge_size" {
  type        = string
  description = "Edge VM size."
  default     = "Standard_D2s_v5"
}

variable "coolify_count" {
  type        = number
  description = "Control-plane VMs: 0 or 1. Stays 0 until wave 3 (DECISIONS I-24)."
  default     = 0
}

variable "coolify_size" {
  type        = string
  description = "Control-plane VM size."
  default     = "Standard_D4s_v5"
}

variable "edge_wireguard_public_key" {
  type        = string
  description = "The edge's WireGuard public key, once the edge exists."
  default     = null
}

variable "snapshots_account_name" {
  type        = string
  description = "Storage account guest snapshots are written to."
}

variable "keyvault_name" {
  type        = string
  description = "Key Vault holding the DEK-wrapping key."
}

variable "manage_dns" {
  type        = bool
  description = "Create the Cloudflare records."
  default     = true
}

variable "cloudflare_zone_id" {
  type        = string
  description = "Cloudflare zone id for herakraft.co."
  default     = null
}

variable "dns_zone" {
  type        = string
  description = "DNS zone the repose names live in."
  default     = "herakraft.co"
}
