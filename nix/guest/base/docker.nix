# Rootful Docker with overlay2 on the ext4 thin volume, compose plugin, and
# an address pool that can never overlap the guest network 10.64.0.0/12
# (docs/workstreams/02-guest-base.md "Docker address pools").
{ config, lib, pkgs, ... }:
{
  virtualisation.docker = {
    enable = true;
    storageDriver = "overlay2";
    enableOnBoot = true;
    autoPrune.enable = false;
    daemon.settings = {
      log-driver = "json-file";
      log-opts = { max-size = "50m"; max-file = "3"; };
      default-address-pools = [
        { base = "172.20.0.0/14"; size = 24; }
      ];
      # The guest has no IPv6 route; Docker's default IPv6 probing only logs.
      ipv6 = false;
    };
  };

  # `docker compose` (the CLI plugin, from nixpkgs docker's compose plugin)
  # and the standalone binary both work.
  environment.systemPackages = [ pkgs.docker-compose ];

  virtualisation.containerd.enable = lib.mkDefault false;
}
