# Edge NixOS configuration: gateway + WireGuard hub.
# See docs/workstreams/06-gateway-edge.md. Skeleton only.
{ config, pkgs, lib, ... }:
{
  imports = [ ./disko.nix ];

  options.repose.edge.osDevice = lib.mkOption {
    type = lib.types.str;
    default = "/dev/sda";
    description = "OS disk for the disko layout: /dev/sda on SCSI sizes (v5), /dev/nvme0n1 on NVMe-only sizes (v6, v7; DECISIONS I-40).";
  };

  config = {
    boot.loader.systemd-boot.enable = true;
    boot.loader.efi.canTouchEfiVariables = true;
    system.stateVersion = "26.11";
    networking.hostName = lib.mkDefault "repose-edge";
    # 22 operator sshd (the user gateway takes it in workstream 06), 443 the
    # preview-proxy stub later and, until the api exists, `hostdev serve`
    # (DECISIONS I-17, I-39). The edge NSG opens the same two.
    networking.firewall.allowedTCPPorts = [ 22 443 ];
    networking.firewall.allowedUDPPorts = [ 51820 ];
    services.openssh.enable = true;
  };
}
