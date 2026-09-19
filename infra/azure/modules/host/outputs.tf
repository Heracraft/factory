output "private_ip" {
  description = "Address hostd, the edge and operators reach this host at. Provider-neutral output."
  value       = azurerm_network_interface.main.private_ip_address
}

output "ssh_jump" {
  description = "How to reach this host over SSH: the jump host and port in front of it. Provider-neutral output."
  value       = "${var.jump_user}@${var.jump_host}:${var.jump_port}"
}

output "vm_id" {
  description = "Azure resource id of the VM."
  value       = azurerm_linux_virtual_machine.main.id
}

output "data_disk_id" {
  description = "Azure resource id of the guest data disk."
  value       = azurerm_managed_disk.data.id
}

output "class" {
  description = "Capacity class recorded on the host."
  value       = var.class
}

output "join_token_delivered" {
  description = "True once a join token has been written to this host by this configuration."
  value       = length(terraform_data.join_token) > 0
}

output "installed" {
  description = "Known once the host is running NixOS and passed the /dev/kvm and nested-virtualization checks."
  value       = module.install.post_install_id
}
