# The on-demand desktop: Xvfb :99, openbox, x11vnc on 127.0.0.1:5900, noVNC
# on 127.0.0.1:6080, socket-activated. A connection to 6080 starts the
# chain through systemd-socket-proxyd; nothing runs and nothing is paid for
# until then. A per-minute check stops the chain after 30 minutes without a
# client, and `systemctl start repose-desktop-idle` stops it now.
#
# The password is generated at every x11vnc start into
# /run/repose/desktop/vnc-password (0600 dev) for the CLI to print
# (docs/features/browser.md); noVNC is only reachable through the SSH
# forward, so the password is defence in depth, not the boundary.
{ config, lib, pkgs, ... }:
let
  display = ":99";
  dir = "/run/repose/desktop";
  # python312 is in the closure already (tools.nix); no second interpreter.
  websockify = pkgs.python312Packages.websockify;
  novncWeb = "${pkgs.novnc}/share/webapps/novnc";
  idleSeconds = 1800;

  genPassword = pkgs.writeShellApplication {
    name = "repose-vnc-password";
    runtimeInputs = [ pkgs.coreutils pkgs.x11vnc ];
    text = ''
      pw=$(head -c 32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 8)
      umask 077
      printf '%s\n' "$pw" > ${dir}/vnc-password
      x11vnc -storepasswd "$pw" ${dir}/vnc-passwd >/dev/null 2>&1
    '';
  };

  idleCheck = pkgs.writeShellApplication {
    name = "repose-desktop-idle-check";
    runtimeInputs = [ pkgs.coreutils pkgs.iproute2 pkgs.systemd ];
    text = ''
      systemctl is-active --quiet repose-xvfb.service || exit 0
      stamp=${dir}/last-client
      clients=$(ss -Htn state established '( sport = :6081 or sport = :5900 )' | wc -l)
      if [ "$clients" -gt 0 ]; then
        touch "$stamp"
        exit 0
      fi
      [ -e "$stamp" ] || touch "$stamp"
      now=$(date +%s); last=$(stat -c %Y "$stamp")
      if [ $((now - last)) -ge ${toString idleSeconds} ]; then
        systemctl start repose-desktop-idle.service
      fi
    '';
  };

  # Wait until a listener is bound, so a unit only counts as started when a
  # dependent can connect (websockify and x11vnc have no sd_notify).
  waitPort = port: pkgs.writeShellScript "repose-wait-${toString port}" ''
    for _ in $(seq 1 100); do
      if ${pkgs.iproute2}/bin/ss -Hltn "sport = :${toString port}" | grep -q LISTEN; then exit 0; fi
      sleep 0.1
    done
    echo "port ${toString port} not listening after 10 s" >&2
    exit 1
  '';

  common = {
    serviceConfig = {
      User = "dev";
      Group = "dev";
      Restart = "no";
    };
    environment.DISPLAY = display;
  };
in
{
  environment.systemPackages = with pkgs; [ xvfb openbox x11vnc websockify novnc ];

  systemd.services.repose-xvfb = lib.recursiveUpdate common {
    description = "repose desktop: Xvfb ${display}";
    serviceConfig = {
      ExecStart = "${pkgs.xvfb}/bin/Xvfb ${display} -screen 0 1600x1000x24 -nolisten tcp -ac -noreset";
      ExecStartPost = "${pkgs.coreutils}/bin/touch ${dir}/last-client";
    };
  };

  systemd.services.repose-openbox = lib.recursiveUpdate common {
    description = "repose desktop: window manager";
    requires = [ "repose-xvfb.service" ];
    after = [ "repose-xvfb.service" ];
    bindsTo = [ "repose-xvfb.service" ];
    serviceConfig.ExecStart = "${pkgs.openbox}/bin/openbox";
    # Xvfb needs a moment to open its socket.
    preStart = ''
      for _ in $(seq 1 50); do
        [ -S /tmp/.X11-unix/X99 ] && exit 0
        sleep 0.1
      done
    '';
  };

  systemd.services.repose-x11vnc = lib.recursiveUpdate common {
    description = "repose desktop: x11vnc on 127.0.0.1:5900";
    requires = [ "repose-xvfb.service" ];
    wants = [ "repose-openbox.service" ];
    after = [ "repose-xvfb.service" "repose-openbox.service" ];
    bindsTo = [ "repose-xvfb.service" ];
    serviceConfig = {
      ExecStartPre = "${genPassword}/bin/repose-vnc-password";
      ExecStart = "${pkgs.x11vnc}/bin/x11vnc -display ${display} -localhost -rfbport 5900 -rfbauth ${dir}/vnc-passwd -forever -shared -noxdamage -quiet";
      ExecStartPost = waitPort 5900;
      # x11vnc exits 2 when told to stop; that is its normal shutdown.
      SuccessExitStatus = "2";
    };
  };

  systemd.services.repose-novnc = lib.recursiveUpdate common {
    description = "repose desktop: noVNC (websockify) on 127.0.0.1:6081";
    requires = [ "repose-x11vnc.service" ];
    after = [ "repose-x11vnc.service" ];
    bindsTo = [ "repose-x11vnc.service" ];
    serviceConfig = {
      ExecStart = "${websockify}/bin/websockify --web ${novncWeb} 127.0.0.1:6081 127.0.0.1:5900";
      ExecStartPost = waitPort 6081;
    };
  };

  systemd.sockets.repose-novnc = {
    description = "repose desktop: noVNC entry point on 127.0.0.1:6080";
    wantedBy = [ "sockets.target" ];
    socketConfig = {
      ListenStream = "127.0.0.1:6080";
      Service = "repose-novnc-proxy.service";
    };
  };

  systemd.services.repose-novnc-proxy = {
    description = "repose desktop: proxy 6080 to noVNC, starting the chain";
    requires = [ "repose-novnc.service" ];
    after = [ "repose-novnc.service" ];
    serviceConfig = {
      ExecStart = "${pkgs.systemd}/lib/systemd/systemd-socket-proxyd --exit-idle-time=60s 127.0.0.1:6081";
      PrivateTmp = true;
      DynamicUser = true;
    };
  };

  systemd.services.repose-desktop-idle = {
    description = "repose desktop: stop the desktop chain";
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${pkgs.systemd}/bin/systemctl stop repose-novnc-proxy.service repose-novnc.service repose-x11vnc.service repose-openbox.service repose-xvfb.service";
    };
  };

  systemd.services.repose-desktop-idle-check = {
    description = "repose desktop: stop after ${toString (idleSeconds / 60)} minutes without a client";
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${idleCheck}/bin/repose-desktop-idle-check";
    };
  };

  systemd.timers.repose-desktop-idle-check = {
    wantedBy = [ "timers.target" ];
    timerConfig = {
      OnBootSec = "1min";
      OnUnitActiveSec = "1min";
      AccuracySec = "10s";
    };
  };

  # dev may start and stop the desktop without a password (repose-guest-profile).
  security.sudo.extraRules = [
    {
      users = [ "dev" ];
      commands = [
        { command = "${pkgs.systemd}/bin/systemctl start repose-novnc.service"; options = [ "NOPASSWD" ]; }
        { command = "${pkgs.systemd}/bin/systemctl start repose-desktop-idle.service"; options = [ "NOPASSWD" ]; }
      ];
    }
  ];
}
