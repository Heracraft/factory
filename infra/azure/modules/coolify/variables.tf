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
  description = "OS disk size. The platform Postgres and every image layer Coolify deploys here live on it."
  default     = 256
}

variable "os_disk_type" {
  type        = string
  description = "OS disk SKU."
  default     = "Premium_LRS"
}

variable "image" {
  description = "Ubuntu LTS image. This VM stays Ubuntu; Coolify rejects NixOS as a managed server (DECISIONS R4-2)."
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

variable "coolify_public_key" {
  type        = string
  description = <<-EOT
    The SSH public key of the owner's existing Coolify instance (Coolify ->
    Keys & Tokens -> Private Keys). Coolify manages this VM as a *server*: it
    connects over SSH as root with this key, installs its proxy and deploys
    the api, the dashboard, Logto and Postgres onto it. Nothing Coolify runs
    here; the instance is the one already on the owner's personal server
    (DECISIONS I-83). A public key, so it belongs in prod.tfvars.
  EOT

  validation {
    condition     = can(regex("^(ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp[0-9]+) [A-Za-z0-9+/=]+", var.coolify_public_key))
    error_message = "coolify_public_key is an OpenSSH public key line (ssh-ed25519 AAAA...), the one Coolify shows under Keys & Tokens."
  }
}

variable "backup_max_age_hours" {
  type        = number
  description = "repose-backup-check reports failure when the newest dump Coolify has written under /data/coolify/backups is older than this. 36 hours: a nightly dump plus a missed night's grace. There is no bucket variable: the destination is an S3 storage in the owner's own Coolify and no credential for it reaches this VM (DECISIONS I-102)."
  default     = 36
}

variable "ssh_private_key_path" {
  type        = string
  description = "Operator private key used by the readiness provisioner. The same key as the first entry of authorized_keys."
}

variable "connect_timeout" {
  type        = string
  description = "How long the readiness provisioner waits for SSH. cloud-init installs Docker from Docker's apt repository first."
  default     = "15m"
}

variable "private_ip" {
  type        = string
  description = <<-EOT
    The VM's VNet address, allocated statically so that hosts can be built
    with it: a host reaches the api's gRPC listener here to register, before
    it has a WireGuard tunnel (DECISIONS I-92). Null keeps Azure's dynamic
    allocation, which is stable in practice but not promised.
  EOT
  default     = null
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
