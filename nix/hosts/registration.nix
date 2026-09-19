# One-shot registration before hostd: consumes /run/repose/join-token
# (written by cloud-init from the OpenTofu output, workstream 11) through
# `hostd register`, which writes host.json, cert.pem and key.pem under
# /var/lib/repose/hostd and deletes the token. Skipped entirely once
# host.json exists or when there is no token (hostd then logs `waiting for
# join token` itself). A token the api rejects as used or invalid makes
# hostd exit 3, and the unit stops retrying; a network error retries every
# 30 seconds.
{ config, lib, pkgs, ... }:
let
  hostd = config.repose.host.hostdPackage;
  stateDir = "/var/lib/repose/hostd";
  token = "/run/repose/join-token";
in
{
  systemd.services.repose-register = {
    description = "Register this host with the api using the join token";
    wantedBy = [ "multi-user.target" ];
    wants = [ "network-online.target" ];
    after = [ "network-online.target" "repose-host-net.service" ]
      ++ lib.optional (config.repose.host.provider == "azure") "cloud-final.service";
    before = [ "hostd.service" ];
    unitConfig.ConditionPathExists = [
      "!${stateDir}/host.json"
      token
    ];
    path = [ pkgs.systemd ];
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${hostd}/bin/hostd register --state ${stateDir} --token ${token}";
      # The bridge, wg0 and sshd read host.json; apply it now.
      ExecStartPost = "${pkgs.systemd}/bin/systemctl --no-block restart repose-host-net.service";
      Restart = "on-failure";
      RestartSec = 30;
      RestartPreventExitStatus = 3;
      StateDirectory = "repose/hostd";
      StateDirectoryMode = "0700";
    };
  };
}
