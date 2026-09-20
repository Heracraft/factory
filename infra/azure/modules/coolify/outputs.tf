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

output "coolify_server" {
  description = <<-EOT
    What the owner's Coolify asks for under Servers -> Add: the address, the
    user and the port it will SSH to, plus the private key to pick (the one
    whose public half is coolify_public_key). Coolify's "Validate & configure"
    then installs its proxy and takes the machine over (docs/ops/coolify.md).
  EOT
  value = {
    ip_address = azurerm_public_ip.main.ip_address
    user       = "root"
    port       = 22
  }
}
