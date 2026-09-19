variable "zone_id" {
  type        = string
  description = "Cloudflare zone id for herakraft.co."
}

variable "records" {
  description = <<-EOT
    Fully qualified name to address. `proxied` must stay false for
    ssh.repose.herakraft.co: Cloudflare's proxy does not carry SSH, and a
    proxied record would hide the edge's real address from every host's
    WireGuard endpoint as well.
  EOT
  type = map(object({
    address = string
    proxied = optional(bool, false)
    comment = optional(string, "managed by repose infra/dns")
  }))

  validation {
    condition = alltrue([
      for name, r in var.records : !r.proxied if startswith(name, "ssh.")
    ])
    error_message = "The ssh. record must not be proxied; Cloudflare's proxy does not carry SSH or WireGuard."
  }
}

variable "ttl" {
  type        = number
  description = "TTL in seconds for unproxied records. Short, because a rebuilt edge or control plane changes an address."
  default     = 300
}
