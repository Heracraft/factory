variable "account_id" {
  type        = string
  description = "Cloudflare account id holding the R2 bucket."
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
