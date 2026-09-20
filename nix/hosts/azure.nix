# Azure specifics, active when repose.host.provider is "azure": the Hyper-V
# drivers, the serial console, waagent so the platform sees VM health (no
# extensions, no provisioning: nixos-anywhere already did that), and
# cloud-init limited to write_files so the join token arrives at
# /run/repose/join-token from the user-data workstream 11 passes. IMDS and
# the wire server are reachable from the host (waagent needs them) and
# blocked from guests by nftables.nix.
{ config, lib, pkgs, ... }:
let
  cfg = config.repose.host;
in
lib.mkIf (cfg.provider == "azure") {
  # v7 sizes are NVMe-only (DECISIONS I-39), and on Azure the NVMe controllers
  # hang off Hyper-V's virtual PCI bus: without pci-hyperv in the initrd the
  # root disk never appears and boot stops in emergency mode waiting for
  # /dev/disk/by-partlabel/disk-os-root (seen on the first edge install).
  # Force-loaded rather than left to udev so the order is not a race.
  boot.initrd.kernelModules = [ "hv_vmbus" "hv_netvsc" "hv_utils" "hv_storvsc" "pci-hyperv" "nvme" ];
  boot.initrd.availableKernelModules = [ "nvme" "pci-hyperv" ];
  boot.kernelParams = [ "console=ttyS0" "earlyprintk=ttyS0" "rootdelay=300" ];
  networking.usePredictableInterfaceNames = false;

  services.waagent = {
    enable = true;
    settings = {
      Provisioning.Enable = false;
      Provisioning.Agent = "disabled";
      ResourceDisk.Format = false;
      Extensions.Enabled = false;
      AutoUpdate.UpdateToLatestVersion = false;
      Logs.Verbose = false;
    };
  };

  services.cloud-init = {
    enable = true;
    network.enable = false;
    settings = {
      datasource_list = [ "Azure" ];
      cloud_init_modules = lib.mkForce [ "write_files" ];
      cloud_config_modules = lib.mkForce [ ];
      cloud_final_modules = lib.mkForce [ ];
      preserve_hostname = lib.mkForce true;
      users = lib.mkForce [ ];
      disable_root = lib.mkForce true;
      network.config = "disabled";
    };
  };

  # cloud-init writes the token into /run/repose; the directory must exist
  # before cloud-init-local runs.
  systemd.services.cloud-init-local.after = [ "systemd-tmpfiles-setup.service" ];

  # Azure data disks by LUN, in case the waagent rules are missing.
  services.udev.extraRules = lib.concatMapStrings (i: ''
    ENV{DEVTYPE}=="disk", KERNEL!="sda" SUBSYSTEM=="block", SUBSYSTEMS=="scsi", KERNELS=="?:0:0:${toString i}", ATTR{removable}=="0", SYMLINK+="disk/by-lun/${toString i}"
  '') (lib.range 0 15);
}
