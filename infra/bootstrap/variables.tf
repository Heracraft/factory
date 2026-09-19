variable "subscription_id" {
  type        = string
  description = "Azure subscription the environment is created in."
}

variable "env" {
  type        = string
  description = "Environment name, recorded in the repose:env tag."
}

variable "resource_group_name" {
  type        = string
  description = "Resource group holding the environment and its state store."
}

variable "location" {
  type        = string
  description = "Azure region."
  default     = "eastus"
}

variable "state_account_name" {
  type        = string
  description = <<-EOT
    Globally unique storage account name for OpenTofu state, or null to create
    only the resource group and keep state in another environment's account
    under a different key. Staging passes null.
  EOT
  default     = null

  validation {
    condition     = var.state_account_name == null || can(regex("^[a-z0-9]{3,24}$", coalesce(var.state_account_name, "xxx")))
    error_message = "Storage account names are 3 to 24 lowercase letters and digits."
  }
}

variable "state_container_name" {
  type        = string
  description = "Container holding the state blobs, one key per root."
  default     = "tfstate"
}

variable "retention_days" {
  type        = number
  description = "Soft-delete window for state blobs and for the container itself."
  default     = 30
}

variable "shared_access_key_enabled" {
  type        = bool
  description = "Whether the account's shared keys work. Off: every backend uses use_azuread_auth."
  default     = false
}

variable "tags" {
  type        = map(string)
  description = "Extra tags."
  default     = {}
}
