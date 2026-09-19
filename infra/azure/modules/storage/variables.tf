variable "account_name" {
  type        = string
  description = "Globally unique storage account name for snapshots. Lowercase letters and digits, 3 to 24 characters."

  validation {
    condition     = can(regex("^[a-z0-9]{3,24}$", var.account_name))
    error_message = "Storage account names are 3 to 24 lowercase letters and digits."
  }
}

variable "container_name" {
  type        = string
  description = "Container guest snapshots are written to."
  default     = "repose-snapshots"
}

variable "host_identity_name" {
  type        = string
  description = "User-assigned managed identity attached to every host VM."
  default     = "id-repose-host"
}

variable "resource_group_name" {
  type        = string
  description = "Resource group the storage account is created in."
}

variable "location" {
  type        = string
  description = "Azure region."
}

variable "tier_to_cool_days" {
  type        = number
  description = "Move a snapshot blob to the cool tier this many days after it was written."
  default     = 7
}

variable "delete_after_days" {
  type        = number
  description = "Delete a snapshot blob this many days after it was written. The backstop behind the api's own retention."
  default     = 45
}

variable "blob_delete_retention_days" {
  type        = number
  description = "Blob soft-delete window. 0 disables it; see the comment in main.tf for why that is the right value here."
  default     = 0
}

variable "container_delete_retention_days" {
  type        = number
  description = "Container soft-delete window. A deleted container is an operator mistake, not a retention promise, so this one is kept."
  default     = 7
}

variable "shared_access_key_enabled" {
  type        = bool
  description = <<-EOT
    Whether the account's shared keys work. hostd authenticates with the
    managed identity, so keys are off; turning them on hands anyone who reads
    one full access to every tenant's snapshots.
  EOT
  default     = false
}

variable "tags" {
  type        = map(string)
  description = "Tags applied to every resource in this module."
  default     = {}
}
