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
    coolify_version       = var.coolify_version
    autoupdate            = var.coolify_autoupdate ? "true" : "false"
    backup_bucket         = var.backup_bucket
    backup_max_age_hours  = var.backup_max_age_hours
    edge_public_key       = var.edge_wireguard_public_key == null ? "" : var.edge_wireguard_public_key
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

# An apply that returns before Coolify is installed leaves the operator with a
# public IP and no way to tell whether cloud-init is still pulling Docker
# images or died twelve minutes ago. The installer itself waits for the
# `coolify` container's Docker health check and exits non-zero if it never
# goes healthy, so re-reading that status here is reading the same signal the
# installer used, not a second invented one.
#
# It is deliberately not tied to custom_data: the VM ignores custom_data
# changes (see the lifecycle block above), so a template edit must not look
# like a reason to re-check a machine that is already serving.
resource "terraform_data" "ready" {
  triggers_replace = {
    vm_id = azurerm_linux_virtual_machine.main.id
  }

  connection {
    type        = "ssh"
    host        = azurerm_public_ip.main.ip_address
    user        = "root"
    port        = 22
    private_key = file(var.ssh_private_key_path)
    timeout     = var.connect_timeout
  }

  provisioner "remote-exec" {
    inline = [
      "cloud-init status --wait >/dev/null 2>&1 || true",
      "cloud-init status | grep -q 'status: done' || { echo 'cloud-init did not finish cleanly on ${var.name}; see /var/log/cloud-init-output.log' >&2; cloud-init status --long >&2; exit 1; }",
      "command -v docker >/dev/null || { echo 'docker is not installed; the Coolify installer did not get that far' >&2; exit 1; }",
      "test \"$(docker inspect --format '{{.State.Health.Status}}' coolify 2>/dev/null)\" = healthy || { echo 'the coolify container is not healthy; run: docker logs coolify' >&2; exit 1; }",
      # The two commands docs/ops/RUNBOOK.md "Postgres restore" opens with.
      # A restore that stops to apt-get something is a restore nobody has
      # rehearsed.
      "command -v rclone >/dev/null && command -v pg_restore >/dev/null || { echo 'rclone or pg_restore missing; the restore procedure cannot be followed on this VM' >&2; exit 1; }",
    ]
  }
}
