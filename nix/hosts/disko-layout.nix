# The disk layout as a pure function of its devices, so the VM test can
# format the data disk alone (`withOs = false`) on a virtual disk while the
# host uses the same code for both disks.
#
# OS disk: GPT, 1 GB ESP, the rest ext4 for `/`.
# Data disk: one PV, VG `vg-guests`, thin pool `thin` on 95 percent of the
# VG. The remaining 5 percent is headroom that lvm.conf's autoextend grows
# the pool into (storage.nix). Pool metadata is 1 percent of the disk,
# clamped to [1 GiB, 16 GiB] (LVM's ceiling), computed in the disko script
# because the disk size is only known at install time.
{
  lib,
  osDevice,
  dataDevice ? null,
  withOs ? true,
  # The edge (nix/edge) has no data disk and reuses only the OS layout.
  withData ? true,
  # Directory of udev rules that create /dev/disk/azure/*; null outside Azure.
  azureUdevRules ? null,
}:
let
  poolMetadataSize = ''"$(( m = $(blockdev --getsize64 "''${lvm_devices[0]}") / 100, m < 1073741824 ? 1073741824 : (m > 17179869184 ? 17179869184 : m) ))b"'';

  # nixos-anywhere's kexec installer has no Azure udev rules, so the
  # /dev/disk/azure/scsi1/lun0 symlink does not exist there yet. Load
  # waagent's rules from the store and retrigger the block devices before
  # touching the data disk. A no-op wherever the symlink already exists.
  azureUdevHook = lib.optionalString (azureUdevRules != null) ''
    if [ ! -e "${dataDevice}" ] && [ -d /run/udev ]; then
      mkdir -p /run/udev/rules.d
      cp ${azureUdevRules}/*.rules /run/udev/rules.d/
      udevadm control --reload
      udevadm trigger --subsystem-match=block --action=add
      udevadm settle
    fi
  '';

  os = {
    type = "disk";
    device = osDevice;
    content = {
      type = "gpt";
      partitions = {
        ESP = {
          size = "1G";
          type = "EF00";
          content = {
            type = "filesystem";
            format = "vfat";
            mountpoint = "/boot";
            mountOptions = [ "umask=0077" ];
          };
        };
        root = {
          size = "100%";
          content = {
            type = "filesystem";
            format = "ext4";
            mountpoint = "/";
          };
        };
      };
    };
  };

  data = {
    type = "disk";
    device = dataDevice;
    preCreateHook = azureUdevHook;
    content = {
      type = "gpt";
      partitions.pv = {
        size = "100%";
        content = {
          type = "lvm_pv";
          vg = "vg-guests";
        };
      };
    };
  };
in
{
  disk = (lib.optionalAttrs withOs { inherit os; }) // (lib.optionalAttrs withData { inherit data; });
}
// lib.optionalAttrs withData {
  lvm_vg.vg-guests = {
    type = "lvm_vg";
    lvs.thin = {
      size = "95%";
      lvm_type = "thin-pool";
      extraArgs = [
        "--poolmetadatasize"
        poolMetadataSize
        # hostd relies on discards reaching the pool so snapshots of trimmed
        # volumes compress to their real contents (03-hostd §5.9).
        "--discards"
        "passdown"
        # Zero new chunks. With zeroing off, a partial first write to a
        # chunk leaves the rest holding whatever the previous owner of that
        # block wrote: a cross-tenant leak through freed blocks.
        "--zero"
        "y"
      ];
    };
  };
}
