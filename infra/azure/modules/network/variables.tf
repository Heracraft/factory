variable "env" {
  description = "Environment name; appears in resource names and in the repose:env tag."
  type        = string
}

variable "resource_group_name" {
  description = "Resource group every network resource is created in."
  type        = string
}

variable "location" {
  description = "Azure region."
  type        = string
}

variable "zone" {
  description = <<-EOT
    Availability zone for zonal resources (NAT gateway public IP). Must be the
    same zone the hosts run in, because a Premium SSD v2 data disk only
    attaches to a VM in its own zone. Null makes them regional.
  EOT
  type        = string
  default     = null
}

variable "vnet_cidr" {
  description = "Address space of the VNet."
  type        = string
  default     = "10.200.0.0/16"
}

variable "hosts_subnet_cidr" {
  description = "Subnet holding repose hosts. No inbound; egress via NAT gateway."
  type        = string
  default     = "10.200.1.0/24"
}

variable "edge_subnet_cidr" {
  description = "Subnet holding the edge VM (SSH gateway and WireGuard hub)."
  type        = string
  default     = "10.200.2.0/24"
}

variable "control_subnet_cidr" {
  description = "Subnet holding the Coolify VM (api, dashboard, Logto, Postgres)."
  type        = string
  default     = "10.200.3.0/24"
}

variable "operator_cidrs" {
  description = <<-EOT
    Source CIDRs allowed to reach operator SSH on the edge and the Coolify VM,
    and the Ubuntu sshd on the edge before nixos-anywhere has run. The owner's
    addresses and the dev box, never 0.0.0.0/0.
  EOT
  type        = list(string)

  validation {
    condition     = length(var.operator_cidrs) > 0 && !contains(var.operator_cidrs, "0.0.0.0/0")
    error_message = "operator_cidrs must be a non-empty list and must not contain 0.0.0.0/0."
  }
}

variable "coolify_manager_cidrs" {
  type        = list(string)
  description = "Addresses the owner's Coolify instance manages the control VM from, over SSH on 22, when not over Tailscale (DECISIONS I-83, I-86). Added to the operator rule; never the whole internet. Empty when Coolify reaches the VM by its tailnet address."
  default     = []

  validation {
    condition     = !contains(var.coolify_manager_cidrs, "0.0.0.0/0")
    error_message = "coolify_manager_cidrs must not contain 0.0.0.0/0; it is the one address the owner's Coolify connects from."
  }
}

variable "control_web_cidrs" {
  description = <<-EOT
    Source CIDRs allowed to reach 80 and 443 on the Coolify VM. Pre-launch this
    is the operator list (workstream 11 §2); at launch it becomes
    ["0.0.0.0/0"], which is also what an ACME http-01 challenge needs. With the
    operator list, Coolify must issue certificates over dns-01.
  EOT
  type        = list(string)
}

variable "edge_operator_ssh_port" {
  description = <<-EOT
    Port the edge's operator sshd listens on. 22 belongs to the repose SSH
    gateway (docs/workstreams/06-gateway-edge.md §5.1).
  EOT
  type        = number
  default     = 2222
}

variable "nat_idle_timeout_minutes" {
  description = "NAT gateway idle timeout. Long-lived agent connections want more than the 4-minute default."
  type        = number
  default     = 30
}

variable "tags" {
  description = "Tags applied to every resource in this module."
  type        = map(string)
  default     = {}
}
