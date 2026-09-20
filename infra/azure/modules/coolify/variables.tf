variable "name" {
  type        = string
  description = "Control-plane VM name."
  default     = "coolify-01"
}

variable "size" {
  type        = string
  description = "Azure VM size for the control plane."
  default     = "Standard_D4s_v7"

  validation {
    # The first production apply died with SkuNotAvailable after creating the
    # NIC and the public IP: this subscription has every v5 and v6
    # general-purpose size marked NotAvailableForSubscription in East US
    # (DECISIONS I-39). A regex cannot know availability, but it can stop the
    # one mistake that has actually happened, which is copying a v5 size name
    # out of an older document.
    condition     = can(regex("^Standard_[A-Z][0-9]+[a-z]*s_v[7-9]$", var.size))
    error_message = "The control-plane size must be a v7 or later Standard size (DECISIONS I-39: v5 and v6 are NotAvailableForSubscription in East US). Confirm with `az vm list-skus --location eastus --size <name> --all` before widening this."
  }
}

variable "zone" {
  type        = string
  description = "Availability zone, or null for regional."
  default     = null
}

variable "resource_group_name" {
  type        = string
  description = "Resource group the VM is created in."
}

variable "location" {
  type        = string
  description = "Azure region."
}

variable "subnet_id" {
  type        = string
  description = "Control subnet; its NSG allows 80, 443 and 22 from the configured CIDRs only."
}

variable "network_ready" {
  description = "Value to depend on so the VM is not created before its subnet's NSG association exists."
  type        = any
  default     = null
}

variable "os_disk_gb" {
  type        = number
  description = "OS disk size. Postgres, the Coolify database and every image layer live here."
  default     = 256
}

variable "os_disk_type" {
  type        = string
  description = "OS disk SKU."
  default     = "Premium_LRS"
}

variable "image" {
  description = "Ubuntu LTS image. This VM stays Ubuntu; Coolify rejects NixOS (DECISIONS R4-2)."
  type = object({
    publisher = string
    offer     = string
    sku       = string
    version   = string
  })
  default = {
    publisher = "Canonical"
    offer     = "ubuntu-24_04-lts"
    sku       = "server"
    version   = "latest"
  }
}

variable "admin_username" {
  type        = string
  description = "Operator user created by cloud-init."
  default     = "azureuser"
}

variable "authorized_keys" {
  type        = list(string)
  description = "Operator public keys."
}

variable "coolify_install_url" {
  type        = string
  description = <<-EOT
    Coolify's installer. Pinning it to a release URL rather than the moving
    `latest` is the difference between a rebuild that reproduces the control
    plane and one that installs whatever shipped this morning.
  EOT
  default     = "https://cdn.coollabs.io/coolify/install.sh"
}

variable "coolify_version" {
  type        = string
  description = <<-EOT
    Coolify release installed at first boot, passed to the installer as its
    one positional argument. Pinned rather than left at the installer's
    `latest`, so that rebuilding this VM reproduces the control plane instead
    of installing whatever shipped that morning. Current releases are listed
    at https://cdn.coollabs.io/coolify/versions.json.
  EOT
  default     = "4.3.23"

  validation {
    condition     = can(regex("^[0-9]+\\.[0-9]+\\.[0-9]+$", var.coolify_version))
    error_message = "coolify_version is an exact release such as 4.3.23; `latest` is what this variable exists to avoid."
  }
}

variable "coolify_autoupdate" {
  type        = bool
  description = <<-EOT
    Let Coolify update itself. False: the control plane runs the api, the
    dashboard, Logto and the platform Postgres, and an unattended upgrade of
    the thing that deploys them is a deploy nobody reviewed. Upgrades are a
    deliberate step in docs/ops/coolify.md.
  EOT
  default     = false
}

variable "backup_bucket" {
  type        = string
  description = "R2 bucket Coolify writes Postgres dumps to (infra/r2 output `bucket_name`). Used only by the repose-backup-check helper; Coolify's own destination is configured in its UI with the token, which is a human step (DECISIONS I-21)."
  default     = "repose-pg-backups"
}

variable "backup_max_age_hours" {
  type        = number
  description = "repose-backup-check reports failure when the newest object in the bucket is older than this. 36 hours: a nightly dump plus a missed night's grace."
  default     = 36
}

variable "ssh_private_key_path" {
  type        = string
  description = "Operator private key used by the readiness provisioner. The same key as the first entry of authorized_keys."
}

variable "connect_timeout" {
  type        = string
  description = "How long the readiness provisioner waits for SSH. Coolify's installer pulls Docker and several images."
  default     = "20m"
}

variable "edge_wireguard_public_key" {
  type        = string
  description = <<-EOT
    The edge's WireGuard public key. Null until the edge has been installed and
    the key read from it (workstream 06 generates it); cloud-init then leaves
    wg0 down and prints this machine's own public key for the operator to add
    as a peer. See infra/README.md, "Wiring the control plane to the edge".
  EOT
  default     = null
}

variable "edge_wireguard_endpoint" {
  type        = string
  description = "host:port of the edge's WireGuard hub."
}

variable "wireguard_address" {
  type        = string
  description = "This machine's address on the edge's WireGuard network. The hosts' pool is 10.255.0.0/16 with the edge at 10.255.0.1."
  default     = "10.255.255.1/16"
}

variable "wireguard_allowed_ips" {
  type        = list(string)
  description = "What the control plane routes over the tunnel: the edge's wg network and every host's guest range."
  default     = ["10.255.0.0/16", "10.64.0.0/12"]
}

variable "tags" {
  type        = map(string)
  description = "Tags applied to every resource in this module."
  default     = {}
}
