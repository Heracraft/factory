# Edge NixOS configuration: gateway + WireGuard hub.
# See docs/workstreams/06-gateway-edge.md. Skeleton only.
{ config, pkgs, lib, ... }:
{
  imports = [ ./disko.nix ];
  boot.loader.systemd-boot.enable = true;
  # Azure guest essentials; the edge is a Standard_D2s_v7 (NVMe-only).
  boot.initrd.kernelModules = [ "hv_vmbus" "hv_netvsc" "hv_utils" "hv_storvsc" ];
  boot.initrd.availableKernelModules = [ "nvme" ];
  boot.kernelParams = [ "console=ttyS0" "earlyprintk=ttyS0" "rootdelay=300" ];
  networking.usePredictableInterfaceNames = false;
  boot.loader.efi.canTouchEfiVariables = true;
  system.stateVersion = "26.11";
  networking.hostName = lib.mkDefault "repose-edge";
  networking.firewall.allowedTCPPorts = [ 22 ];
  networking.firewall.allowedUDPPorts = [ 51820 ];
  services.openssh.enable = true;
}
