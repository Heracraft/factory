output "account_name" {
  description = "Snapshot storage account name."
  value       = azurerm_storage_account.snapshots.name
}

output "account_id" {
  description = "Snapshot storage account resource id."
  value       = azurerm_storage_account.snapshots.id
}

output "container_name" {
  description = "Snapshot container name."
  value       = azurerm_storage_container.snapshots.name
}

output "blob_endpoint" {
  description = "Base URL hostd writes snapshots to."
  value       = azurerm_storage_account.snapshots.primary_blob_endpoint
}

output "host_identity_id" {
  description = "Resource id of the host managed identity; goes on every host VM."
  value       = azurerm_user_assigned_identity.host.id
}

output "host_identity_client_id" {
  description = "Client id hostd passes to the Azure SDK to pick this identity."
  value       = azurerm_user_assigned_identity.host.client_id
}

output "host_identity_principal_id" {
  description = "Object id of the host identity, for role assignments elsewhere."
  value       = azurerm_user_assigned_identity.host.principal_id
}
