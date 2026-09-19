# Disk layout for nixos-anywhere; the shape lives in disko-layout.nix.
# Both devices are options so workstream 11 can pass what it attached. A
# missing data disk makes disko fail with `device ... not found` before it
# touches the OS disk (01-host-nixos §6).
{ config, lib, pkgs, ... }:
let
  cfg = config.repose.host;
in
{
  disko.devices = import ./disko-layout.nix {
    inherit lib;
    inherit (cfg) osDevice dataDevice;
    azureUdevRules = if cfg.provider == "azure" then "${pkgs.waagent}/etc/udev/rules.d" else null;
  };
}
