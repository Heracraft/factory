# Host NixOS configuration. See docs/workstreams/01-host-nixos.md and
# docs/interfaces/host-conventions.md. Skeleton only.
{ config, pkgs, lib, ... }:
{
  imports = [ ./disko.nix ];
  boot.loader.systemd-boot.enable = true;
  boot.loader.efi.canTouchEfiVariables = true;
  system.stateVersion = "26.11";
  networking.hostName = lib.mkDefault "repose-host";
  boot.kernelModules = [ "kvm-intel" "vhost_vsock" "nf_tables" ];
  virtualisation.libvirtd.enable = false;
  environment.systemPackages = with pkgs; [ cloud-hypervisor virtiofsd lvm2 nftables wireguard-tools ];
  services.openssh.enable = true;
  services.openssh.settings.PasswordAuthentication = false;
}
