# Cloudflare R2: where Coolify's nightly Postgres dumps go.
#
# DECISIONS R4-14: Coolify only backs up to S3-compatible targets, and putting
# a translation proxy in the backup path means the backup path is the thing
# that breaks. R2 is the S3-compatible target, and it is deliberately at a
# different provider from everything else, so losing the Azure subscription
# does not lose the backups of the database that describes it.
#
# The bucket and its lifecycle rule are declared here. The API token Coolify
# authenticates with is not: it is a credential, it would live in this state
# file in clear text for the life of the bucket, and it is already a human
# step in docs/ops/AZURE-SETUP.md step 10 (DECISIONS I-21).

terraform {
  required_version = ">= 1.6.0"

  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = ">= 5.0, < 6.0"
    }
  }

  backend "azurerm" {
    resource_group_name  = "repose-prod"
    storage_account_name = "reposetfstate3912"
    container_name       = "tfstate"
    key                  = "r2.tfstate"
    use_azuread_auth     = true
  }
}

# Token from the CLOUDFLARE_API_TOKEN environment variable.
provider "cloudflare" {}

resource "cloudflare_r2_bucket" "pg_backups" {
  account_id    = var.account_id
  name          = var.bucket_name
  location      = var.location
  storage_class = "Standard"

  lifecycle {
    prevent_destroy = true
  }
}

# 35 days: a month of dailies plus slack, so a restore rehearsal that starts
# from "the dump from five weeks ago" still has something to start from.
resource "cloudflare_r2_bucket_lifecycle" "pg_backups" {
  account_id  = var.account_id
  bucket_name = cloudflare_r2_bucket.pg_backups.name

  rules = [
    {
      id      = "expire-old-dumps"
      enabled = true

      conditions = {
        prefix = ""
      }

      delete_objects_transition = {
        condition = {
          type    = "Age"
          max_age = var.retention_days * 24 * 60 * 60
        }
      }

      abort_multipart_uploads_transition = {
        condition = {
          type    = "Age"
          max_age = var.abort_multipart_after_days * 24 * 60 * 60
        }
      }
    },
  ]
}
