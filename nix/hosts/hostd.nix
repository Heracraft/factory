# The hostd unit, the guests slice, the nightly snapshot timer, and the
# state directories from host-conventions.md. hostd restarts at any time
# without touching guests: they are transient units in guests.slice, not
# its children (KillMode=process), and it reconciles from bbolt, LVM and
# `systemctl list-units 'guest@*'` at start (03-hostd).
{ config, lib, pkgs, ... }:
let
  cfg = config.repose.host;
  hostd = cfg.hostdPackage;
  apiFlags = "--api-addr ${lib.escapeShellArg cfg.apiAddr}"
    + lib.optionalString (cfg.apiServerName != "") " --api-server-name ${lib.escapeShellArg cfg.apiServerName}"
    + lib.optionalString (cfg.apiCA != "") " --api-ca ${pkgs.writeText "repose-api-ca.pem" cfg.apiCA}";
  # hostd refuses to start without a snapshot target (03-hostd §5.9): Blob
  # when the host has an account, its own disk otherwise.
  snapshotFlags =
    if cfg.snapshots.blobUrl != "" then
      "--blob-url ${lib.escapeShellArg cfg.snapshots.blobUrl} --blob-container ${lib.escapeShellArg cfg.snapshots.container}"
      + lib.optionalString (cfg.snapshots.identityClientId != "") " --blob-identity ${lib.escapeShellArg cfg.snapshots.identityClientId}"
    else
      "--snapshot-dir ${lib.escapeShellArg cfg.snapshots.localDir}";
  stateDir = "/var/lib/repose/hostd";

  # guests.slice gets everything but the host reserve: 8 GiB below 128 GiB
  # of RAM, 16 GiB above (DESIGN §4). Computed at boot; the slice cap is the
  # hard line that keeps hostd, virtiofsd and builds alive if the api's
  # reservation accounting is ever wrong.
  guestsSlice = pkgs.writeShellApplication {
    name = "repose-guests-slice";
    runtimeInputs = [ pkgs.systemd pkgs.gawk ];
    text = ''
      total=$(awk '/^MemTotal:/ { print $2 * 1024 }' /proc/meminfo)
      gib=$((1024 * 1024 * 1024))
      if [ "$total" -lt $((128 * gib)) ]; then reserve=$((8 * gib)); else reserve=$((16 * gib)); fi
      max=$((total - reserve))
      if [ "$max" -le 0 ]; then max=$((total / 2)); fi
      systemctl set-property --runtime guests.slice MemoryMax="$max"
      echo "guests.slice MemoryMax=$max (total=$total reserve=$reserve)"
    '';
  };
in
{
  systemd.slices.guests = {
    description = "Tenant guests (Cloud Hypervisor and virtiofsd transient units)";
    sliceConfig = {
      CPUWeight = 100;
      MemoryAccounting = true;
      CPUAccounting = true;
      IOAccounting = true;
    };
  };

  systemd.services.repose-guests-slice = {
    description = "Set guests.slice memory cap from the host's RAM";
    wantedBy = [ "multi-user.target" ];
    before = [ "hostd.service" ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStart = "${guestsSlice}/bin/repose-guests-slice";
    };
  };

  systemd.services.hostd = {
    description = "repose hostd: runs tenants' guests on this host";
    wantedBy = [ "multi-user.target" ];
    wants = [
      "network-online.target"
      "repose-host-net.service"
      "repose-register.service"
      "repose-store-export.service"
      "repose-guests-slice.service"
    ];
    after = [
      "network-online.target"
      "nftables.service"
      "repose-host-net.service"
      "repose-register.service"
      "repose-store-export.service"
      "repose-guests-slice.service"
      "lvm2-monitor.service"
    ];
    path = with pkgs; [
      lvm2
      thin-provisioning-tools
      nftables
      iproute2
      util-linux
      e2fsprogs
      systemd
      nix
      cloud-hypervisor
      virtiofsd
      zstd
      coreutils
    ];
    serviceConfig = {
      ExecStart = "${hostd}/bin/hostd --state ${stateDir} ${apiFlags} ${snapshotFlags}";
      Restart = "always";
      RestartSec = 2;
      # A join token that has been used is not retried (03-hostd §6).
      RestartPreventExitStatus = 3;
      # Guests are transient units, not children: a hostd restart or crash
      # must not stop them.
      KillMode = "process";
      LimitNOFILE = 1048576;
      StateDirectory = "repose/hostd repose/guests repose/builds";
      StateDirectoryMode = "0700";
      LogsDirectory = "repose";
      OOMScoreAdjust = -900;
      TimeoutStopSec = 30;
    };
  };

  systemd.services.repose-snapshot = {
    description = "Nightly snapshot of every running guest";
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${hostd}/bin/hostd snapshot-all";
    };
  };

  systemd.timers.repose-snapshot = {
    description = "Nightly snapshot at 03:00 local time";
    wantedBy = [ "timers.target" ];
    timerConfig = {
      OnCalendar = "*-*-* 03:00:00";
      # A missed night is what the SnapshotStale alert is for; a snapshot
      # storm at boot is not wanted.
      Persistent = false;
      RandomizedDelaySec = "10min";
      Unit = "repose-snapshot.service";
    };
  };

  systemd.tmpfiles.rules = [
    "d /var/lib/repose 0755 root root -"
    "d /var/lib/repose/guests 0700 root root -"
    "d /var/lib/repose/builds 0700 root root -"
    "d /var/log/repose 0750 root root -"
  ] ++ lib.optional (cfg.snapshots.blobUrl == "") "d ${cfg.snapshots.localDir} 0700 root root -";
}
