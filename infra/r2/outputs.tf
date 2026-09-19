output "bucket_name" {
  description = "Bucket name to configure in Coolify's backup destination."
  value       = cloudflare_r2_bucket.pg_backups.name
}

output "s3_endpoint" {
  description = "S3-compatible endpoint Coolify points at."
  value       = "https://${var.account_id}.r2.cloudflarestorage.com"
}
