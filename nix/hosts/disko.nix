# Disk layout for nixos-anywhere. OS disk only; the guest thin pool lives on
# the attached managed data disk and is created by 01-host-nixos.
{ ... }:
{
  disko.devices.disk.os = {
    type = "disk";
    device = "/dev/sda";
    content = {
      type = "gpt";
      partitions = {
        ESP = { size = "512M"; type = "EF00"; content = { type = "filesystem"; format = "vfat"; mountpoint = "/boot"; }; };
        root = { size = "100%"; content = { type = "filesystem"; format = "ext4"; mountpoint = "/"; }; };
      };
    };
  };
}
