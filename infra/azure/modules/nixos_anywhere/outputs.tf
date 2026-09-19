output "installed_id" {
  description = "Depend on this to order work after the machine is running NixOS."
  value       = terraform_data.install.id
}

output "post_install_id" {
  description = "Depend on this to order work after post_install_commands have run."
  value       = length(terraform_data.post_install) > 0 ? terraform_data.post_install[0].id : terraform_data.install.id
}
