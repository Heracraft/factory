# Edge NixOS configuration: gateway + WireGuard hub.
# See docs/workstreams/06-gateway-edge.md. Skeleton only.
{ config, pkgs, lib, ... }:
{
  imports = [ ../hosts/disko.nix ];
  boot.loader.systemd-boot.enable = true;
  boot.loader.efi.canTouchEfiVariables = true;
  system.stateVersion = "26.11";
  networking.hostName = lib.mkDefault "repose-edge";
  networking.firewall.allowedTCPPorts = [ 22 ];
  networking.firewall.allowedUDPPorts = [ 51820 ];
  services.openssh.enable = true;
}
