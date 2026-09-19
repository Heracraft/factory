# Cloudflare account id is an identifier, not a credential; the token is in
# the CLOUDFLARE_API_TOKEN environment variable. Fill this in from the R2
# dashboard (docs/ops/AZURE-SETUP.md step 10).

# account_id = "0123456789abcdef0123456789abcdef"

bucket_name    = "repose-pg-backups"
location       = "ENAM"
retention_days = 35
