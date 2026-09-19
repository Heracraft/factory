variable "name" {
  type        = string
  description = "Control-plane VM name."
  default     = "coolify-01"
}

variable "size" {
  type        = string
  description = "Azure VM size for the control plane."
  default     = "Standard_D4s_v5"
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
