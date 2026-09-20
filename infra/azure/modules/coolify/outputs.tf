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

output "dashboard_url" {
  description = <<-EOT
    Coolify's own dashboard. It listens on 8000 over plain HTTP and the
    control subnet NSG does not open that port on purpose, so this is the
    address to use *after* `ssh -L 8000:127.0.0.1:8000 root@<public ip>`
    (docs/ops/coolify.md).
  EOT
  value       = "http://127.0.0.1:8000"
}

output "version" {
  description = "Coolify release this VM was installed with."
  value       = var.coolify_version
}
