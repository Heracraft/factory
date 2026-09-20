variable "account_id" {
  type        = string
  description = "Cloudflare account id holding the R2 bucket. An identifier, not a credential; the token is in CLOUDFLARE_API_TOKEN (DECISIONS I-21)."

  validation {
    condition     = var.account_id != null && can(regex("^[0-9a-f]{32}$", var.account_id))
    error_message = "account_id is the 32-character Cloudflare account id from the R2 dashboard; fill it in in r2.tfvars (docs/ops/AZURE-SETUP.md step 10)."
  }
}

variable "bucket_name" {
  type        = string
  description = "Bucket Coolify writes Postgres dumps to."
  default     = "repose-pg-backups"
}

variable "location" {
  type        = string
  description = "R2 location hint. ENAM keeps the dumps near the East US control plane."
  default     = "ENAM"
}

variable "retention_days" {
  type        = number
  description = "Delete a dump this many days after it was written."
  default     = 35
}

variable "abort_multipart_after_days" {
  type        = number
  description = "Abort a multipart upload left incomplete this many days. A half-uploaded dump is billed storage that no restore can use."
  default     = 7
}
