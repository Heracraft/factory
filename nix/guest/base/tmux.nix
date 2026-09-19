# System tmux config and the per-project session unit.
#
# The session is created by a *user* unit of `dev`, not by guestd, so it
# belongs to dev's tmux server and outlives guestd restarts. It waits (path
# unit) for /home/dev/.repose/project.json, which guestd writes at
# SetupProject, then creates the session named after the slug with window
# `shell` in /home/dev/<slug>. Creating it twice is a no-op.
{ config, lib, pkgs, ... }:
let
  tmuxSession = pkgs.writeShellApplication {
    name = "repose-tmux-session";
    runtimeInputs = [ pkgs.tmux pkgs.jq pkgs.coreutils ];
    text = ''
      project="$HOME/.repose/project.json"
      if [ ! -s "$project" ]; then
        echo "repose-tmux-session: $project missing; nothing to do" >&2
        exit 0
      fi
      slug=$(jq -r '.slug // empty' "$project")
      if [ -z "$slug" ]; then
        echo "repose-tmux-session: project.json has no slug" >&2
        exit 1
      fi
      dir="$HOME/$slug"
      mkdir -p "$dir"
      if tmux has-session -t "=$slug" 2>/dev/null; then
        exit 0
      fi
      # -d: detached. The server keeps running in this unit's cgroup.
      tmux new-session -d -s "$slug" -n shell -c "$dir"
    '';
  };
in
{
  programs.tmux = {
    enable = true;
    # One socket path for everyone: /tmp/tmux-1000/default. With the secure
    # socket the server started by the user unit and a client in an SSH
    # session would look in different places (TMUX_TMPDIR is only set in
    # login shells).
    secureSocket = false;
    # Written to /etc/tmux.conf; guest-conventions.md "tmux" lists these.
    historyLimit = 50000;
    escapeTime = 10;
    terminal = "tmux-256color";
    extraConfig = ''
      set -g set-clipboard on
      set -g mouse on
      set -ga terminal-overrides ",*:Tc"
      set -g focus-events on
      set -g default-shell ${pkgs.bash}/bin/bash
      # Env the CLI sets on the SSH session should reach new windows.
      set -g update-environment "DISPLAY SSH_AUTH_SOCK SSH_CONNECTION TZ LANG COLORTERM"
    '';
  };

  environment.systemPackages = [ tmuxSession ];

  systemd.user.services.repose-tmux-session = {
    description = "repose: project tmux session";
    after = [ "default.target" ];
    unitConfig.ConditionPathExists = "%h/.repose/project.json";
    serviceConfig = {
      Type = "forking";
      ExecStart = "${tmuxSession}/bin/repose-tmux-session";
      # tmux server exits when the last session is killed; that is a clean
      # stop, not a failure.
      Restart = "no";
      KillMode = "control-group";
    };
  };

  systemd.user.paths.repose-tmux-session = {
    description = "repose: start the project tmux session once project.json exists";
    wantedBy = [ "default.target" ];
    pathConfig = {
      PathExists = "%h/.repose/project.json";
      Unit = "repose-tmux-session.service";
    };
  };

  systemd.tmpfiles.rules = [
    "d /home/dev/.repose 0700 dev dev -"
  ];
}
