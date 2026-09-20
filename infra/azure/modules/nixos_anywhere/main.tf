# Turn a freshly booted Ubuntu VM into the NixOS system named by a flake
# attribute, using nixos-anywhere over SSH.
#
# Ubuntu is a delivery vehicle only (docs/workstreams/11-infra-opentofu.md §5,
# "Why Ubuntu as the install target"): nixos-anywhere kexecs into the NixOS
# installer, runs disko against nix/hosts/disko.nix, installs the closure and
# reboots. Nothing from the Ubuntu system survives, which is why the operator
# public keys are handed to the new system through --extra-files rather than
# being left in cloud-init's authorized_keys.
#
# Anything the installed system needs that is *not* in the flake goes through
# post_install_commands, which run over SSH after the reboot. The join token
# takes that path (DECISIONS I-20): a token in cloud-init would sit in the
# Azure VM model and in IMDS for the life of the VM, and would be wiped by the
# install anyway.

terraform {
  required_version = ">= 1.6.0"
}

locals {
  ssh_common = concat(
    [
      "StrictHostKeyChecking=no",
      "UserKnownHostsFile=/dev/null",
      "ConnectTimeout=10",
    ],
    var.jump_host == null ? [] : [
      "ProxyJump=${var.jump_user}@${var.jump_host}:${var.jump_port}",
    ],
  )

  ssh_option_flags = join(" ", [for o in local.ssh_common : "--ssh-option ${o}"])
  ssh_cli_flags    = join(" ", [for o in local.ssh_common : "-o ${o}"])

  authorized_keys = join("\n", var.authorized_keys)
}

# Wait until cloud-init has finished on the Ubuntu image, then put the
# operator keys on root ourselves. Connecting as root at this point is a race
# the first apply lost: until cloud-init's ssh module has run, root's
# authorized_keys is the image's forced-command line that refuses everything
# with "Please login as the user azureuser", and remote-exec treats that as a
# hard failure rather than a retry. The provisioning user is always let in,
# and installing the keys here also repairs a VM whose cloud-init ran with an
# older template.
resource "terraform_data" "wait_for_cloud_init" {
  triggers_replace = var.triggers

  connection {
    type                = "ssh"
    host                = var.target_host
    user                = var.bootstrap_user
    port                = 22
    private_key         = file(var.ssh_private_key_path)
    timeout             = var.connect_timeout
    bastion_host        = var.jump_host
    bastion_user        = var.jump_host == null ? null : var.jump_user
    bastion_port        = var.jump_host == null ? null : var.jump_port
    bastion_private_key = var.jump_host == null ? null : file(var.ssh_private_key_path)
  }

  provisioner "file" {
    content     = "${local.authorized_keys}\n"
    destination = "/tmp/repose-root-authorized_keys"
  }

  provisioner "remote-exec" {
    inline = [
      "sudo cloud-init status --wait >/dev/null 2>&1 || true",
      "sudo install -d -m 0700 -o root -g root /root/.ssh",
      "sudo install -m 0600 -o root -g root /tmp/repose-root-authorized_keys /root/.ssh/authorized_keys",
      "rm -f /tmp/repose-root-authorized_keys",
      "sudo test -s /root/.ssh/authorized_keys",
      "! sudo grep -q 'Please login as the user' /root/.ssh/authorized_keys",
      "sudo systemctl restart ssh || sudo systemctl restart sshd",
    ]
  }
}

resource "terraform_data" "install" {
  triggers_replace = var.triggers

  depends_on = [terraform_data.wait_for_cloud_init]

  provisioner "local-exec" {
    interpreter = ["/usr/bin/env", "bash", "-euo", "pipefail", "-c"]
    environment = {
      REPOSE_AUTHORIZED_KEYS = local.authorized_keys
    }
    command = <<-EOT
      extra=$(mktemp -d)
      trap 'rm -rf "$extra"' EXIT
      mkdir -p "$extra/root/.ssh"
      printf '%s\n' "$REPOSE_AUTHORIZED_KEYS" > "$extra/root/.ssh/authorized_keys"
      chmod 700 "$extra/root/.ssh"
      chmod 600 "$extra/root/.ssh/authorized_keys"

      nixos-anywhere \
        --flake '${var.flake_path}#${var.flake_attr}' \
        --extra-files "$extra" \
        --chown /root/.ssh 0:0 \
        --build-on ${var.build_on} \
        -i '${var.ssh_private_key_path}' \
        ${local.ssh_option_flags} \
        --post-kexec-ssh-port 22 \
        '${var.target_user}@${var.target_host}'
    EOT
  }
}

# Everything after the reboot into NixOS. The connection port is the installed
# system's sshd port, which is not 22 on the edge (22 is the user gateway).
resource "terraform_data" "post_install" {
  count = length(var.post_install_commands) > 0 ? 1 : 0

  triggers_replace = [var.triggers, var.post_install_triggers]

  depends_on = [terraform_data.install]

  connection {
    type                = "ssh"
    host                = var.target_host
    user                = "root"
    port                = var.post_install_port
    private_key         = file(var.ssh_private_key_path)
    timeout             = var.connect_timeout
    bastion_host        = var.jump_host
    bastion_user        = var.jump_host == null ? null : var.jump_user
    bastion_port        = var.jump_host == null ? null : var.jump_port
    bastion_private_key = var.jump_host == null ? null : file(var.ssh_private_key_path)
  }

  provisioner "remote-exec" {
    inline = concat(
      [
        # Fail loudly if the reboot came up on something other than NixOS:
        # a half-finished kexec leaves Ubuntu running and every later step
        # would silently configure the wrong system.
        "grep -q '^ID=nixos' /etc/os-release || { echo 'nixos-anywhere did not leave NixOS on ${var.name}' >&2; exit 1; }",
      ],
      var.post_install_commands,
    )
  }
}
