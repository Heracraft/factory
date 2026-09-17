# Platform guest base module. See docs/workstreams/02-guest-base.md and
# docs/interfaces/guest-conventions.md. Skeleton only.
{ config, pkgs, lib, ... }:
{
  imports = [ ./tools.nix ];
  system.stateVersion = "26.11";
  users.users.dev = {
    isNormalUser = true;
    uid = 1000;
    extraGroups = [ "wheel" "docker" ];
  };
  security.sudo.wheelNeedsPassword = false;
  virtualisation.docker.enable = true;
  programs.tmux.enable = true;
  services.openssh.enable = true;
  services.openssh.settings = {
    PasswordAuthentication = false;
    PermitRootLogin = "no";
  };
  boot.kernel.sysctl = {
    "fs.inotify.max_user_watches" = 1048576;
    "fs.inotify.max_user_instances" = 1024;
  };
}
