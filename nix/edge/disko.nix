# Edge disk layout: the OS disk half of the host layout, no data disk.
# Workstream 06 owns the edge; this keeps `nixos-anywhere --flake .#edge`
# working after the host layout gained its own options.
{ lib, ... }:
{
  disko.devices = import ../hosts/disko-layout.nix {
    inherit lib;
    osDevice = "/dev/sda";
    withData = false;
  };
}
