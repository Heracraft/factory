output "resource_group_name" {
  description = "Pass this to the environment roots as resource_group_name."
  value       = azurerm_resource_group.main.name
}

output "backend_config" {
  description = "The backend block the environment roots need, or null when this environment borrows another's state account."
  value = var.state_account_name == null ? null : {
    resource_group_name  = azurerm_resource_group.main.name
    storage_account_name = azurerm_storage_account.state[0].name
    container_name       = azurerm_storage_container.tfstate[0].name
    use_azuread_auth     = true
  }
}
