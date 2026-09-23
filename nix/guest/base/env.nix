# Environment every process of dev's sees (guest-conventions.md
# "Environment"). Static values are NixOS environment.sessionVariables,
# which reach both /etc/set-environment (every bash, login or not) and
# /etc/pam/environment (the SSH session and dev's systemd user manager, so
# user units and the tmux server it starts). TZ and REPOSE_PROJECT come
# from /etc/repose/env (written by guestd at SetupProject) and named
# secrets from /run/repose/secrets.env, both sourced by
# /etc/profile.d/repose.sh, which also exports DISPLAY while the desktop
# is up. That file is sourced from every shell through extraInit so login
# and interactive shells behave the same.
{ config, lib, pkgs, ... }:
let
  home = "/home/dev";
  # Every package manager's user bin dir (DECISIONS I-227), ahead of the
  # system profile so a user's install wins over the base's copy.
  userBinDirs = map (d: "${home}/${d}") (import ./user-bin-dirs.nix);
in
{
  environment.sessionVariables = {
    LANG = "C.UTF-8";
    EDITOR = "nvim";
    REPOSE = "1";
    COLORTERM = "truecolor";
    # The nodejs store path is read-only; global installs need a prefix.
    NPM_CONFIG_PREFIX = "${home}/.npm-global";
    PNPM_HOME = "${home}/.local/share/pnpm";
    # The tools' own defaults, set so every process (user units too)
    # agrees on them.
    GOPATH = "${home}/go";
    CARGO_HOME = "${home}/.cargo";
    RUSTUP_HOME = "${home}/.rustup";
    BUN_INSTALL = "${home}/.bun";
    DENO_INSTALL_ROOT = "${home}/.deno";
    COMPOSER_HOME = "${home}/.config/composer";
    # Not a default: without it `gem install` writes to ruby's store path.
    GEM_HOME = "${home}/.local/share/gem";
    PATH = userBinDirs;
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
  '';

  environment.extraInit = ''
    if [ -r /etc/profile.d/repose.sh ]; then
      . /etc/profile.d/repose.sh
    fi
  '';

  # A base applied without a reboot changes /etc, but two long-lived
  # processes keep the PATH they started with: dev's tmux server (a
  # `tmux new-window <cmd>`, how `repose run` starts an agent, runs `bash
  # -c` with the server's environment) and dev's user manager (what a user
  # unit starts with). Give both the new login PATH. Idempotent; nothing
  # to do on first boot, when neither is running yet.
  system.activationScripts.repose-user-path = {
    deps = [ "etc" "users" ];
    text = ''
      if [ -S /tmp/tmux-1000/default ] || [ -S /run/user/1000/bus ]; then
        ${pkgs.util-linux}/bin/runuser -u dev -- ${pkgs.coreutils}/bin/env -i \
          HOME=/home/dev USER=dev LOGNAME=dev XDG_RUNTIME_DIR=/run/user/1000 \
          ${pkgs.bash}/bin/bash -lc '
            if [ -S /tmp/tmux-1000/default ]; then
              ${pkgs.tmux}/bin/tmux -S /tmp/tmux-1000/default set-environment -g PATH "$PATH" 2>/dev/null || true
            fi
            if [ -S /run/user/1000/bus ]; then
              ${pkgs.systemd}/bin/systemctl --user set-environment PATH="$PATH" 2>/dev/null || true
            fi
          ' || true
      fi
    '';
  };

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
