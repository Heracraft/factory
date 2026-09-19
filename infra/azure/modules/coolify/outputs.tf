output "public_ip" {
  description = "Static address the dashboard, api and Logto hostnames point at."
  value       = azurerm_public_ip.main.ip_address
}

output "private_ip" {
  description = "Control-plane address inside the VNet."
  value       = azurerm_network_interface.main.private_ip_address
}

output "vm_id" {
  description = "Azure resource id of the control-plane VM."
  value       = azurerm_linux_virtual_machine.main.id
}
