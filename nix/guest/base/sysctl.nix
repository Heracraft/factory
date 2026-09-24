# Kernel tunables for agents that watch large trees and builds that spike.
# zram gives a build that briefly exceeds RAM somewhere to go other than the
# OOM killer picking the agent.
{ config, lib, pkgs, ... }:
{
  boot.kernel.sysctl = {
    "fs.inotify.max_user_watches" = 1048576;
    "fs.inotify.max_user_instances" = 1024;
    "fs.file-max" = 2097152;
    "net.core.somaxconn" = 4096;
    "vm.swappiness" = 10;
  };

  # The same swap zramSwap made (zstd, half the RAM up to 2 GiB, priority 5),
  # set up by a unit of ours instead of zram-generator's (DECISIONS I-231).
  # The generator's swap is part of swap.target, every tmpfs mount is
  # ordered after swap.target, and so /run/wrappers, local-fs.target,
  # sysinit.target and everything after them waited for the zram device to
  # appear, be formatted and be swapped on: 0.6 s of every boot on the dev
  # box. Nothing needs swap in the first seconds of a boot.
  systemd.services.repose-zram-swap = {
    description = "repose: compressed swap on zram";
    wantedBy = [ "multi-user.target" ];
    unitConfig.DefaultDependencies = false;
    after = [ "local-fs.target" ];
    before = [ "shutdown.target" ];
    conflicts = [ "shutdown.target" ];
    path = [ pkgs.util-linux pkgs.kmod pkgs.gawk ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };
    script = ''
      if grep -q '^/dev/zram0 ' /proc/swaps; then
        exit 0
      fi
      modprobe zram
      ram=$(awk '/^MemTotal:/ { print $2 * 1024 }' /proc/meminfo)
      size=$((ram / 2))
      max=$((2 * 1024 * 1024 * 1024))
      [ "$size" -le "$max" ] || size=$max
      zramctl --algorithm zstd --size "$size" /dev/zram0
      mkswap -L zram0 /dev/zram0 >/dev/null
      swapon --priority 5 --discard /dev/zram0
    '';
    preStop = ''
      swapoff /dev/zram0 || true
    '';
  };
}
