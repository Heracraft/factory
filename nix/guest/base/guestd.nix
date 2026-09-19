# guestd: the in-guest daemon (workstream 04). This module only runs it and
# prepares the paths it needs (guest-conventions.md "Filesystem"):
# /run/repose (tmpfs, part of /run), /run/repose/secrets (0700 dev), and the
# hooks socket directory. guestd creates hooks.sock itself. The binary comes
# from the flake's packages.guestd through `repose.guestd.package`.
{ config, lib, pkgs, ... }:
{
  options.repose.guestd.package = lib.mkOption {
    type = lib.types.package;
    description = "The guestd binary package (flake packages.guestd).";
  };

  config = {
    systemd.tmpfiles.rules = [
      "d /run/repose 0755 root root -"
      "d /run/repose/secrets 0700 dev dev -"
      "d /run/repose/desktop 0700 dev dev -"
    ];

    systemd.services.guestd = {
      description = "repose guestd (vsock control agent)";
      wantedBy = [ "multi-user.target" ];
      # Before sshd so Ready is sent by something that saw sshd start; after
      # the tmpfiles rules so /run/repose exists.
      before = [ "sshd.service" ];
      after = [ "systemd-tmpfiles-setup.service" "network.target" "docker.service" ];
      wants = [ "docker.service" ];
      environment.GOMAXPROCS = "1";
      serviceConfig = {
        ExecStart = "${config.repose.guestd.package}/bin/guestd";
        Restart = "always";
        RestartSec = "2s";
        # Logging goes to the serial console (journal → console) so a frozen
        # root filesystem never blocks guestd's own writes.
        StandardOutput = "journal+console";
        StandardError = "journal+console";
        OOMScoreAdjust = -900;
        KillSignal = "SIGTERM";
      };
    };

    environment.systemPackages = [ config.repose.guestd.package ];
  };
}
