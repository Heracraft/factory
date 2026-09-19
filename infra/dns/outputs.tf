output "fqdns" {
  description = "The names created, in the order they were given."
  value       = { for k, r in cloudflare_dns_record.a : k => r.content }
}
