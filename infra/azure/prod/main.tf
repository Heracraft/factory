# repose production.
#
# Everything this environment is made of lives in ../modules/environment; this
# root pins the state key, configures the providers, and supplies the values
# that are true of production and nowhere else.
#
# Plan and apply through the Makefile: `make plan ENV=prod`, `make apply ENV=prod`.

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

  # Created by hand on 2026-09-19 (docs/ops/AZURE-SETUP.md step 5) and
  # described by infra/bootstrap. Never managed from here: an apply that could
  # destroy this container could destroy the state describing it.
  # Locking is by blob lease; `tofu force-unlock` is in infra/README.md.
  backend "azurerm" {
    resource_group_name  = "repose-prod"
    storage_account_name = "reposetfstate3912"
    container_name       = "tfstate"
    key                  = "azure.tfstate"
    use_azuread_auth     = true
  }
}

provider "azurerm" {
  features {}
  subscription_id = var.subscription_id
}

# Token from the CLOUDFLARE_API_TOKEN environment variable. It is scoped to
# DNS edit on herakraft.co and is never written to a file in this repository.
provider "cloudflare" {}

module "environment" {
  source = "../modules/environment"

  env                 = "prod"
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
  host_size         = var.host_size
  host_class        = var.host_class
  host_data_disk_gb = var.host_data_disk_gb

  # Written out rather than defaulted: the portal's default is Trusted Launch,
  # which silently disables nested virtualization (docs/DESIGN.md §4).
  host_security_type = "Standard"

  edge_size    = var.edge_size
  coolify_size = var.coolify_size

  edge_wireguard_public_key = var.edge_wireguard_public_key

  snapshots_account_name = var.snapshots_account_name
  keyvault_name          = var.keyvault_name

  manage_dns         = var.manage_dns
  cloudflare_zone_id = var.cloudflare_zone_id
  dns_zone           = var.dns_zone
  dns_prefix         = "repose"
}
