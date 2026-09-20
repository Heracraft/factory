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

    # The guest's nix database does not know the paths it sees through the
    # shared store until hostd sends RegisterPaths after Ready (DECISIONS
    # I-55). home-manager's activation is the first thing at boot that asks
    # nix about them, so it waits for guestd's stamp; a guest whose hostd
    # never comes proceeds after the timeout and the unit fails as before.
    systemd.services.repose-paths = {
      description = "Wait for hostd to register the shared store paths";
      wantedBy = [ "multi-user.target" ];
      after = [ "guestd.service" ];
      wants = [ "guestd.service" ];
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
        ExecStart = "${pkgs.writeShellScript "repose-paths-wait" ''
          for _ in $(seq 1 360); do
            [ -e /run/repose/paths-registered ] && exit 0
            sleep 0.5
          done
          echo "no path registration from hostd after 180 s; continuing"
        ''}";
      };
    };
    systemd.services.home-manager-dev = {
      after = [ "repose-paths.service" ];
      wants = [ "repose-paths.service" ];
    };

    environment.systemPackages = [ config.repose.guestd.package ];
  };
}
