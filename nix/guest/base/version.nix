# The base version stamp. /etc/repose/base-version is what `repose status`
# compares with the api's base_versions row; the NixOS label makes
# `nixos-version` inside the guest say the same thing.
{ config, lib, ... }:
{
  environment.etc."repose/base-version".text = config.repose.baseVersion + "\n";
  system.nixos.label = config.repose.baseVersion;
  system.nixos.tags = [ "repose" ];
}
