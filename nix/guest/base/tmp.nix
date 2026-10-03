# /tmp is on the guest's volume and starts every boot empty, but not through
# boot.tmp.cleanOnBoot (DECISIONS I-410): its `D! /tmp` rule has
# systemd-tmpfiles-setup delete the old /tmp file by file before
# sysinit.target, and guestd, sshd and everything else waited for it: 18 to
# 20 s of a 27 s start on host-01, for a /tmp that a day of go test and
# browsers had filled. Here the old /tmp is renamed aside in one step before
# tmpfiles runs (tmp.conf's `q /tmp` creates the new one), and deleted a
# minute after the boot at idle priority.
{ pkgs, ... }:
let
  # Earlier boots' /tmp, each moved here whole.
  trash = "/var/lib/repose/tmp-old";
in
{
  boot.tmp.cleanOnBoot = false;

  systemd.services.repose-tmp-rotate = {
    description = "repose: set the last boot's /tmp aside";
    wantedBy = [ "sysinit.target" ];
    unitConfig = {
      DefaultDependencies = false;
      # A /tmp mounted over (a tmpfs a user configured) has nothing to clear.
      ConditionPathIsMountPoint = "!/tmp";
    };
    after = [ "systemd-remount-fs.service" ];
    before = [ "systemd-tmpfiles-setup.service" "sysinit.target" "shutdown.target" ];
    conflicts = [ "shutdown.target" ];
    serviceConfig.Type = "oneshot";
    path = [ pkgs.coreutils pkgs.systemd ];
    # A base applied without a reboot (guestd's Switch) restarts the active
    # targets, and sysinit.target would start this unit on a running
    # machine, under its tmux and browsers. It runs only in a boot, before
    # systemd-tmpfiles-setup.
    script = ''
      if systemctl is-active --quiet systemd-tmpfiles-setup.service; then
        exit 0
      fi
      [ -d /tmp ] || exit 0
      [ -n "$(ls -A /tmp)" ] || exit 0
      install -d -m 0700 ${trash}
      mv /tmp "$(mktemp -d -p ${trash})/tmp"
      install -d -m 1777 /tmp
    '';
  };

  systemd.services.repose-tmp-purge = {
    description = "repose: delete the last boot's /tmp";
    unitConfig.ConditionDirectoryNotEmpty = trash;
    serviceConfig = {
      Type = "oneshot";
      Nice = 19;
      IOSchedulingClass = "idle";
      ExecStart = "${pkgs.findutils}/bin/find ${trash} -mindepth 1 -delete";
    };
  };
  systemd.timers.repose-tmp-purge = {
    wantedBy = [ "timers.target" ];
    timerConfig.OnBootSec = "1min";
  };
}
