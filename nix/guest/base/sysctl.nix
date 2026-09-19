# Kernel tunables for agents that watch large trees and builds that spike.
# zram gives a build that briefly exceeds RAM somewhere to go other than the
# OOM killer picking the agent.
{ config, lib, ... }:
{
  boot.kernel.sysctl = {
    "fs.inotify.max_user_watches" = 1048576;
    "fs.inotify.max_user_instances" = 1024;
    "fs.file-max" = 2097152;
    "net.core.somaxconn" = 4096;
    "vm.swappiness" = 10;
  };

  zramSwap = {
    enable = true;
    algorithm = "zstd";
    memoryPercent = 50;
    memoryMax = 2 * 1024 * 1024 * 1024;
  };
}
