# Environment every shell in the guest sees (guest-conventions.md
# "Environment"). Static values are NixOS environment.variables; TZ and
# REPOSE_PROJECT come from /etc/repose/env (written by guestd at
# SetupProject) and named secrets from /run/repose/secrets.env, both sourced
# by /etc/profile.d/repose.sh, which also exports DISPLAY while the desktop
# is up. That file is sourced from every shell through extraInit so login
# and interactive shells behave the same.
{ config, lib, pkgs, ... }:
{
  environment.variables = {
    LANG = "C.UTF-8";
    EDITOR = "nvim";
    REPOSE = "1";
    COLORTERM = "truecolor";
    NPM_CONFIG_PREFIX = "/home/dev/.npm-global";
    PNPM_HOME = "/home/dev/.local/share/pnpm";
  };

  environment.etc."profile.d/repose.sh".text = ''
    # repose guest profile: sourced by every shell (see nix/guest/base/env.nix).
    if [ -r /etc/repose/env ]; then
      set -a
      . /etc/repose/env
      set +a
    fi
    if [ -r /run/repose/secrets.env ]; then
      . /run/repose/secrets.env
    fi
    # DISPLAY only while the on-demand desktop's X server is up.
    if [ -S /tmp/.X11-unix/X99 ]; then
      export DISPLAY=:99
    else
      unset DISPLAY
    fi
    case ":$PATH:" in
      *":/home/dev/.local/bin:"*) ;;
      *) export PATH="/home/dev/.local/bin:/home/dev/.local/share/pnpm:/home/dev/.npm-global/bin:$PATH" ;;
    esac
  '';

  environment.extraInit = ''
    if [ -r /etc/profile.d/repose.sh ]; then
      . /etc/profile.d/repose.sh
    fi
  '';

  systemd.tmpfiles.rules = [
    "d /etc/repose 0755 root root -"
    "d /home/dev/.local 0755 dev dev -"
    "d /home/dev/.local/bin 0755 dev dev -"
    "d /home/dev/.local/share 0755 dev dev -"
    "d /home/dev/.local/share/pnpm 0755 dev dev -"
    "d /home/dev/.npm-global 0755 dev dev -"
    "d /home/dev/.npm-global/bin 0755 dev dev -"
  ];
}
