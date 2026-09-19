# Host hardening. One human account (root, via the Host CA or the bootstrap
# key), no sudo, tmpfs /tmp, bounded journal, weekly fstrim, and no
# automatic reboots: an unattended reboot kills every tenant, so kernel
# updates are applied by draining a host (workstream 03 owns Drain).
{ config, lib, pkgs, ... }:
{
  security.sudo.enable = false;
  security.polkit.enable = lib.mkDefault false;

  users.mutableUsers = false;
  users.users.root.hashedPassword = "!";
  users.users.root.openssh.authorizedKeys.keys =
    lib.mkIf config.repose.host.bootstrap.enable config.repose.host.bootstrap.authorizedKeys;

  # hostd loads tun devices and Cloud Hypervisor opens /dev/kvm after boot;
  # the module list in kernel.nix is the control instead.
  security.lockKernelModules = false;

  boot.tmp.useTmpfs = true;
  boot.tmp.cleanOnBoot = true;

  services.journald.settings.Journal = {
    SystemMaxUse = "4G";
    RateLimitIntervalSec = "30s";
    RateLimitBurst = 20000;
  };

  # Root has no password and, outside bootstrap, no static key: logins are
  # Host CA certificates (ssh.nix), which NixOS cannot see at build time.
  users.allowNoPasswordLogin = true;

  services.fstrim.enable = true;
  services.fstrim.interval = "weekly";

  system.autoUpgrade.enable = false;
  # A watchdog reboot is an unattended reboot; both stay at their default (off).
  systemd.settings.Manager.RuntimeWatchdogSec = "off";
  systemd.settings.Manager.RebootWatchdogSec = "off";

  # Nothing on a host reads documentation or needs a shell for a tenant.
  documentation.enable = false;
  documentation.nixos.enable = false;
  programs.command-not-found.enable = false;
}
