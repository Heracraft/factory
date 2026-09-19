# The fixed guest user. Claude Code refuses --dangerously-skip-permissions
# as root, MCP configs key on absolute paths, and every doc says
# /home/dev/<slug>, so `dev` at uid 1000 is not optional (DECISIONS R3-13).
{ config, lib, pkgs, ... }:
{
  users.mutableUsers = false;
  # Access is certificate-only through the platform CA (ssh.nix); there is
  # deliberately no password anywhere in the guest.
  users.allowNoPasswordLogin = true;

  users.groups.dev.gid = 1000;
  users.users.dev = {
    isNormalUser = true;
    uid = 1000;
    group = "dev";
    home = "/home/dev";
    createHome = true;
    shell = pkgs.bash;
    extraGroups = [ "wheel" "docker" "kvm" ];
    # A user unit (repose-tmux-session) must run without a login session.
    linger = true;
    # No password: SSH is certificate-only and sudo needs none.
    hashedPassword = "!";
  };

  users.users.root = {
    # Locked: `passwd -S root` shows L. Root is reached by `sudo -i` only.
    hashedPassword = "!";
  };

  security.sudo.enable = true;
  security.sudo.wheelNeedsPassword = false;
  security.sudo.execWheelOnly = true;
}
