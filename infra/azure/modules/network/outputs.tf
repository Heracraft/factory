output "vnet_id" {
  description = "Resource id of the VNet."
  value       = azurerm_virtual_network.main.id
}

output "vnet_name" {
  description = "Name of the VNet."
  value       = azurerm_virtual_network.main.name
}

output "hosts_subnet_id" {
  description = "Subnet repose hosts are placed in."
  value       = azurerm_subnet.hosts.id
}

output "edge_subnet_id" {
  description = "Subnet the edge VM is placed in."
  value       = azurerm_subnet.edge.id
}

output "control_subnet_id" {
  description = "Subnet the Coolify VM is placed in."
  value       = azurerm_subnet.control.id
}

output "hosts_nsg_id" {
  description = "NSG on the hosts subnet. Has no security rules by design."
  value       = azurerm_network_security_group.hosts.id
}

output "nat_public_ip" {
  description = "Address every guest on every host egresses from."
  value       = azurerm_public_ip.nat.ip_address
}

output "hosts_subnet_ready" {
  description = <<-EOT
    Depend on this before creating a host: it is only known once the NSG and
    NAT gateway associations exist, so a host cannot come up on a subnet that
    is briefly unprotected or has no egress.
  EOT
  value = join(",", [
    azurerm_subnet_network_security_group_association.hosts.id,
    azurerm_subnet_nat_gateway_association.hosts.id,
  ])
}

output "control_subnet_ready" {
  description = <<-EOT
    Depend on this before creating the control plane: it is only known once
    the control subnet's NSG association exists. It matters more than it looks
    — the VM's readiness provisioner SSHes in over the public IP, and a subnet
    whose NSG arrives mid-apply drops that connection rather than refusing it,
    which reads as a twenty-minute hang.
  EOT
  value       = azurerm_subnet_network_security_group_association.control.id
}
