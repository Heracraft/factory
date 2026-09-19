# repose staging.
#
# The same composition as prod, with its own state key and its own names, so
# that a change to infra/ can be planned and rehearsed before it touches the
# environment holding tenants. CI plans this root with hosts = [], which costs
# nothing because a plan creates nothing.
#
# Its resource group is created by `make bootstrap ENV=staging`; the state
# account is prod's, under a different key, because a second state account
# would be a second thing to remember to protect.
#
# Plan and apply through the Makefile: `make plan ENV=staging`.

terraform {
  required_version = ">= 1.6.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = ">= 4.0, < 5.0"
    }
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = ">= 5.0, < 6.0"
    }
  }

  # Prod's state account (docs/ops/AZURE-SETUP.md step 5), under staging's own
  # key. Never managed from here.
  backend "azurerm" {
    resource_group_name  = "repose-prod"
    storage_account_name = "reposetfstate3912"
    container_name       = "tfstate"
    key                  = "staging.tfstate"
    use_azuread_auth     = true
  }
}

provider "azurerm" {
  features {}
  subscription_id = var.subscription_id

  # The subscription's resource providers were registered by hand
  # (docs/ops/AZURE-SETUP.md step 3). Letting the provider re-check them on
  # every plan adds minutes of polling against Azure Resource Manager for a
  # question whose answer never changes. A new subscription registers them
  # once, by following that step.
  resource_provider_registrations = "none"
}

# Token from the CLOUDFLARE_API_TOKEN environment variable. It is scoped to
# DNS edit on herakraft.co and is never written to a file in this repository.
provider "cloudflare" {}

module "environment" {
  source = "../modules/environment"

  env                 = "staging"
  resource_group_name = var.resource_group_name
  zone                = var.zone

  operator_cidrs    = var.operator_cidrs
  control_web_cidrs = var.control_web_cidrs

  operator_authorized_keys = var.operator_authorized_keys
  ssh_private_key_path     = var.ssh_private_key_path
  api_identity_object_id   = var.api_identity_object_id

  flake_path = var.flake_path
  build_on   = var.build_on

  hosts       = var.hosts
  join_tokens = var.join_tokens

  # DECISIONS I-14: the pre-launch host is a D16s_v5 with a 512 GB data disk.
  # Launch values are Standard_D64s_v5 and 2048, changed here and applied with
  # guests stopped (deallocate, resize, start).
  host_size           = var.host_size
  host_class          = var.host_class
  host_data_disk_gb   = var.host_data_disk_gb
  host_data_disk_iops = var.host_data_disk_iops
  host_data_disk_mbps = var.host_data_disk_mbps

  # Comes from the environment's tfvars, which is where somebody reads it.
  # The variable has no default, so a plan without that file fails rather than
  # silently picking a security type (docs/workstreams/11-infra-opentofu.md §6).
  host_security_type = var.host_security_type

  edge_size    = var.edge_size
  coolify_size = var.coolify_size

  edge_wireguard_public_key = var.edge_wireguard_public_key

  snapshots_account_name = var.snapshots_account_name
  keyvault_name          = var.keyvault_name

  manage_dns         = var.manage_dns
  cloudflare_zone_id = var.cloudflare_zone_id
  dns_zone           = var.dns_zone
  dns_prefix         = var.dns_prefix
}
