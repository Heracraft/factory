variable "name" {
  description = "Machine name, used only in error messages."
  type        = string
}

variable "flake_path" {
  description = "Absolute path to the directory holding nix/flake.nix."
  type        = string
}

variable "flake_attr" {
  description = "nixosConfigurations attribute to install, e.g. \"host\" or \"edge\"."
  type        = string
}

variable "target_host" {
  description = "Address nixos-anywhere connects to. A private address when jump_host is set."
  type        = string
}

variable "target_user" {
  description = "User on the Ubuntu image nixos-anywhere logs in as. Must be root."
  type        = string
  default     = "root"

  validation {
    condition     = var.target_user == "root"
    error_message = "nixos-anywhere needs root on the install target; cloud-init opens it for exactly this step."
  }
}

variable "ssh_private_key_path" {
  description = "Operator private key matching one of authorized_keys. Never read into state."
  type        = string
}

variable "authorized_keys" {
  description = "Operator public keys written to /root/.ssh/authorized_keys on the installed system."
  type        = list(string)

  validation {
    condition     = length(var.authorized_keys) > 0
    error_message = "At least one operator public key is required or the installed system is unreachable."
  }
}

variable "jump_host" {
  description = "Address of the SSH jump host, or null to connect directly. Hosts have no public IP, so they are reached through the edge."
  type        = string
  default     = null
}

variable "jump_user" {
  description = "User on the jump host."
  type        = string
  default     = "root"
}

variable "jump_port" {
  description = "SSH port on the jump host. The edge's operator sshd is not on 22."
  type        = number
  default     = 2222
}

variable "post_install_port" {
  description = "sshd port of the installed NixOS system."
  type        = number
  default     = 22
}

variable "post_install_commands" {
  description = "Commands run over SSH after the reboot into NixOS."
  type        = list(string)
  default     = []
}

variable "post_install_triggers" {
  description = "Values that re-run post_install_commands without reinstalling the machine (the join token uses this)."
  type        = any
  default     = null
}

variable "build_on" {
  description = "nixos-anywhere --build-on: auto, local or remote."
  type        = string
  default     = "auto"

  validation {
    condition     = contains(["auto", "local", "remote"], var.build_on)
    error_message = "build_on must be auto, local or remote."
  }
}

variable "connect_timeout" {
  description = "How long a provisioner waits for SSH. A kexec plus disko plus install is minutes, not seconds."
  type        = string
  default     = "20m"
}

variable "triggers" {
  description = "Values that force a reinstall when they change (VM id, flake attribute)."
  type        = any
}

variable "bootstrap_user" {
  description = "The image's provisioning user, which cloud images always let in. Used only to wait for cloud-init and to put the operator keys on root before nixos-anywhere logs in as target_user."
  type        = string
  default     = "azureuser"
}
