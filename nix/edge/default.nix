# Edge NixOS configuration: gateway + WireGuard hub.
# See docs/workstreams/06-gateway-edge.md. Skeleton only.
{ config, pkgs, lib, ... }:
{
  imports = [ ./disko.nix ];

  options.repose.edge.osDevice = lib.mkOption {
    type = lib.types.str;
    default = "/dev/nvme0n1";
    description = "OS disk for the disko layout: /dev/nvme0n1 on the NVMe-only v7 sizes (DECISIONS I-39); /dev/sda on a SCSI size.";
  };

  config = {
    boot.loader.systemd-boot.enable = true;
    # Azure guest essentials; the edge is a Standard_D2s_v7 (NVMe-only).
    boot.initrd.kernelModules = [ "hv_vmbus" "hv_netvsc" "hv_utils" "hv_storvsc" ];
    boot.initrd.availableKernelModules = [ "nvme" ];
    boot.kernelParams = [ "console=ttyS0" "earlyprintk=ttyS0" "rootdelay=300" ];
    networking.usePredictableInterfaceNames = false;
    boot.loader.efi.canTouchEfiVariables = true;
    system.stateVersion = "26.11";
    networking.hostName = lib.mkDefault "repose-edge";
    # 22 operator sshd (the user gateway takes it in workstream 06), 443 the
    # preview-proxy stub later and, until the api exists, `hostdev serve`
    # (DECISIONS I-17, I-40). The edge NSG opens the same two.
    networking.firewall.allowedTCPPorts = [ 22 443 ];
    networking.firewall.allowedUDPPorts = [ 51820 ];
    services.openssh.enable = true;
  };
}
