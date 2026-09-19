# Where guest snapshots live, and the identity hosts write them with.
#
# docs/DESIGN.md §6: a snapshot is an LVM thin snapshot streamed zstd-
# compressed to <user>/<project>/<timestamp>.img.zst in the repose-snapshots
# container. Retention is the api's job; the lifecycle rule here is the
# backstop for anything the api forgets.

terraform {
  required_version = ">= 1.6.0"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = ">= 4.0, < 5.0"
    }
  }
}

locals {
  tags = merge(var.tags, { "repose:role" = "snapshots" })
}

resource "azurerm_storage_account" "snapshots" {
  name                = var.account_name
  resource_group_name = var.resource_group_name
  location            = var.location

  account_tier             = "Standard"
  account_replication_type = "LRS"
  account_kind             = "StorageV2"
  access_tier              = "Hot"

  https_traffic_only_enabled      = true
  min_tls_version                 = "TLS1_2"
  allow_nested_items_to_be_public = false
  shared_access_key_enabled       = var.shared_access_key_enabled

  # Hosts have no public IP and egress through the NAT gateway, so they reach
  # Blob over the internet like any other client.
  public_network_access_enabled = true

  blob_properties {
    # Deliberately no versioning and no delete-retention on this account.
    # DESIGN §6 and DECISIONS R4-11 promise that a destroyed project's last
    # snapshot is kept 30 days and then deleted, and that a cancelled
    # account's snapshots are deleted after 30 days. A soft-delete window
    # would keep tenant data past the day the policy says it is gone, which
    # is a broken promise rather than a safety net.
    versioning_enabled = false

    # Azure has no "0 days"; the policy is expressed by the block's absence.
    dynamic "delete_retention_policy" {
      for_each = var.blob_delete_retention_days > 0 ? [var.blob_delete_retention_days] : []
      content {
        days = delete_retention_policy.value
      }
    }

    dynamic "container_delete_retention_policy" {
      for_each = var.container_delete_retention_days > 0 ? [var.container_delete_retention_days] : []
      content {
        days = container_delete_retention_policy.value
      }
    }
  }

  lifecycle {
    prevent_destroy = true
  }

  tags = local.tags
}

resource "azurerm_storage_container" "snapshots" {
  name                  = var.container_name
  storage_account_id    = azurerm_storage_account.snapshots.id
  container_access_type = "private"

  lifecycle {
    prevent_destroy = true
  }
}

# Backstop for the api's own retention. 45 days is the 30-day post-destroy
# window plus slack (docs/workstreams/11-infra-opentofu.md §2), so a blob this
# rule deletes is one the api already should have.
resource "azurerm_storage_management_policy" "snapshots" {
  storage_account_id = azurerm_storage_account.snapshots.id

  rule {
    name    = "snapshots-tier-and-expire"
    enabled = true

    filters {
      blob_types   = ["blockBlob"]
      prefix_match = ["${var.container_name}/"]
    }

    actions {
      base_blob {
        tier_to_cool_after_days_since_creation_greater_than = var.tier_to_cool_days
        delete_after_days_since_creation_greater_than       = var.delete_after_days
      }
    }
  }
}

# hostd authenticates to Blob as this identity, assigned to every host VM.
# Scoped to the one container: a host that is compromised reads and writes
# snapshots and nothing else in the subscription.
resource "azurerm_user_assigned_identity" "host" {
  name                = var.host_identity_name
  resource_group_name = var.resource_group_name
  location            = var.location
  tags                = merge(var.tags, { "repose:role" = "host" })
}

resource "azurerm_role_assignment" "host_snapshots" {
  scope                = azurerm_storage_container.snapshots.id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = azurerm_user_assigned_identity.host.principal_id
}
