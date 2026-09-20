output "bucket_name" {
  description = "Bucket name to configure in Coolify's backup destination."
  value       = cloudflare_r2_bucket.pg_backups.name
}

output "s3_endpoint" {
  description = "S3-compatible endpoint Coolify points at."
  value       = "https://${var.account_id}.r2.cloudflarestorage.com"
}

# Coolify's S3 storage form has five fields and gets them wrong in two
# predictable ways: the endpoint must carry the scheme, and the region must be
# the literal `auto` rather than blank or an AWS region name. Printing the
# whole form is how "add the backup destination" stops being a step somebody
# has to reconstruct (docs/ops/coolify.md).
output "coolify_s3_destination" {
  description = "Exactly what to type into Coolify's S3 storage form; the access key and secret come from the R2 API token, which is a human step."
  value = {
    name        = "r2-pg-backups"
    endpoint    = "https://${var.account_id}.r2.cloudflarestorage.com"
    bucket      = cloudflare_r2_bucket.pg_backups.name
    region      = "auto"
    access_key  = "<from the R2 API token, docs/ops/AZURE-SETUP.md step 10>"
    secret_key  = "<from the R2 API token, docs/ops/AZURE-SETUP.md step 10>"
    retention   = "${var.retention_days} days, enforced by the bucket lifecycle rule as well as by Coolify"
    rclone_hint = "rclone config create r2 s3 provider=Cloudflare endpoint=https://${var.account_id}.r2.cloudflarestorage.com access_key_id=... secret_access_key=..."
  }
}
