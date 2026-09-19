output "vault_uri" {
  description = "Base URI the api passes to the Azure Key Vault SDK."
  value       = azurerm_key_vault.main.vault_uri
}

output "vault_id" {
  description = "Azure resource id of the vault."
  value       = azurerm_key_vault.main.id
}

output "key_id" {
  description = "Versioned id of the wrapping key. The api stores this next to each wrapped DEK so a rotation is a re-wrap, not a re-encrypt."
  value       = azurerm_key_vault_key.dek_wrap.id
}

output "key_versionless_id" {
  description = "Version-independent id of the wrapping key."
  value       = azurerm_key_vault_key.dek_wrap.versionless_id
}

output "api_policy_configured" {
  description = "False when api_identity_object_id was not supplied, so the api cannot wrap or unwrap yet."
  value       = var.api_identity_object_id != null
}
