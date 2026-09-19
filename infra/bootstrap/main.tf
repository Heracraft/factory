# The one root applied with local state: the resource group an environment
# lives in and the storage account holding every other root's state.
#
# It is applied once per environment and then left alone (DECISIONS I-18).
# Production's resource group and state account were created by hand on
# 2026-09-19 following docs/ops/AZURE-SETUP.md steps 4 and 5; this module
# describes exactly that shape so a second environment is one apply rather
# than a remembered sequence of portal clicks, and so the settings the state
# store depends on (versioning, soft delete, no public access) are written
# down somewhere reviewable.
#
# Adopting production's existing resources instead of creating new ones:
#
#   tofu -chdir=infra/bootstrap import \
#     azurerm_resource_group.main \
#     /subscriptions/<sub>/resourceGroups/repose-prod
#   tofu -chdir=infra/bootstrap import \
#     azurerm_storage_account.state \
#     /subscriptions/<sub>/resourceGroups/repose-prod/providers/Microsoft.Storage/storageAccounts/reposetfstate3912
#
# Do that only if you want production's state store managed from here. It is
# deliberately not, today: a corrupted bootstrap state would then be able to
# destroy the state of every other root.

terraform {
  required_version = ">= 1.6.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = ">= 4.0, < 5.0"
    }
  }

  # No backend block: this root's own state is a local file. There is nowhere
  # remote to put it, because it is what creates the remote place.
}

provider "azurerm" {
  features {}
  subscription_id     = var.subscription_id
  storage_use_azuread = true

  # See the note in infra/azure/prod/main.tf; docs/ops/AZURE-SETUP.md step 3
  # registers the providers once, by hand.
  resource_provider_registrations = "none"
}

locals {
  tags = merge(var.tags, {
    "repose:env"        = var.env
    "repose:role"       = "tfstate"
    "repose:managed-by" = "opentofu-bootstrap"
  })
}

resource "azurerm_resource_group" "main" {
  name     = var.resource_group_name
  location = var.location
  tags     = local.tags

  lifecycle {
    # Deleting this resource group deletes the environment and the state that
    # describes it, in that order, with nothing left to plan against.
    prevent_destroy = true
  }
}

# Created only when state_account_name is set. Staging reuses production's
# account under its own key, because a second state account is a second thing
# to remember to protect and a second thing to lose.
resource "azurerm_storage_account" "state" {
  count = var.state_account_name == null ? 0 : 1

  name                = var.state_account_name
  resource_group_name = azurerm_resource_group.main.name
  location            = azurerm_resource_group.main.location

  account_tier             = "Standard"
  account_replication_type = "LRS"
  account_kind             = "StorageV2"
  access_tier              = "Hot"

  https_traffic_only_enabled      = true
  min_tls_version                 = "TLS1_2"
  allow_nested_items_to_be_public = false

  # Every backend block sets use_azuread_auth, so nothing needs a shared key,
  # and a shared key is a credential that grants full access to every
  # environment's state to whoever finds it.
  shared_access_key_enabled = var.shared_access_key_enabled

  blob_properties {
    # A state file is the only description of what exists. Versioning is what
    # turns "the apply corrupted the state" into "restore the previous blob
    # version" (docs/workstreams/11-infra-opentofu.md §8).
    versioning_enabled = true

    delete_retention_policy {
      days = var.retention_days
    }

    container_delete_retention_policy {
      days = var.retention_days
    }
  }

  tags = local.tags

  lifecycle {
    prevent_destroy = true
  }
}

resource "azurerm_storage_container" "tfstate" {
  count = var.state_account_name == null ? 0 : 1

  name                  = var.state_container_name
  storage_account_id    = azurerm_storage_account.state[0].id
  container_access_type = "private"

  lifecycle {
    prevent_destroy = true
  }
}
