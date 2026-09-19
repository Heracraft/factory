# The control-plane VM: Coolify itself, Logto, Postgres, and the api and
# dashboard as single-container Coolify applications (DECISIONS R4-2, R5-11).
#
# Unlike the host and the edge this VM stays Ubuntu and is never touched by
# nixos-anywhere, because Coolify rejects NixOS as a host or managed server.

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
  tags = merge(var.tags, { "repose:role" = "control" })
}

resource "azurerm_public_ip" "main" {
  name                = "pip-${var.name}"
  resource_group_name = var.resource_group_name
  location            = var.location
  allocation_method   = "Static"
  sku                 = "Standard"
  zones               = var.zone == null ? null : [var.zone]
  tags                = local.tags

  lifecycle {
    # repose.herakraft.co, api.repose.herakraft.co and auth.repose.herakraft.co
    # all resolve here; a replacement is a DNS outage plus a Logto issuer change.
    prevent_destroy = true
  }
}

resource "azurerm_network_interface" "main" {
  name                = "nic-${var.name}"
  resource_group_name = var.resource_group_name
  location            = var.location
  tags                = local.tags

  ip_configuration {
    name                          = "public"
    subnet_id                     = var.subnet_id
    private_ip_address_allocation = "Dynamic"
    public_ip_address_id          = azurerm_public_ip.main.id
  }
}

resource "azurerm_linux_virtual_machine" "main" {
  name                = var.name
  computer_name       = var.name
  resource_group_name = var.resource_group_name
  location            = var.location
  size                = var.size
  zone                = var.zone
  tags                = local.tags

  network_interface_ids = [azurerm_network_interface.main.id]

  admin_username                  = var.admin_username
  disable_password_authentication = true

  admin_ssh_key {
    username   = var.admin_username
    public_key = var.authorized_keys[0]
  }

  secure_boot_enabled = false
  vtpm_enabled        = false

  custom_data = base64encode(templatefile("${path.module}/templates/cloud-init.yaml.tftpl", {
    authorized_keys       = join("\n", var.authorized_keys)
    coolify_install_url   = var.coolify_install_url
    edge_public_key       = coalesce(var.edge_wireguard_public_key, "")
    edge_endpoint         = var.edge_wireguard_endpoint
    wireguard_address     = var.wireguard_address
    wireguard_allowed_ips = join(", ", var.wireguard_allowed_ips)
  }))

  os_disk {
    name                 = "osdisk-${var.name}"
    caching              = "ReadWrite"
    storage_account_type = var.os_disk_type
    disk_size_gb         = var.os_disk_gb
  }

  source_image_reference {
    publisher = var.image.publisher
    offer     = var.image.offer
    sku       = var.image.sku
    version   = var.image.version
  }

  boot_diagnostics {}

  lifecycle {
    # Postgres and every Coolify application definition live on this VM's OS
    # disk. Re-imaging it because the marketplace published a new Ubuntu
    # version, or because a WireGuard key changed in cloud-init, would destroy
    # the control plane. Rebuilding it is a deliberate, documented restore
    # from the R2 dump (infra/README.md).
    ignore_changes = [source_image_reference, custom_data]

    precondition {
      condition     = length(var.authorized_keys) > 0
      error_message = "The Coolify VM needs at least one operator public key; there is no other way in."
    }
  }
}
