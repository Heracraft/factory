# The on-demand desktop: Xvfb :99 (1440x900), openbox, x11vnc on
# 127.0.0.1:5900, noVNC on 127.0.0.1:6080, socket-activated. The display
# has two users (DECISIONS I-246): the agents' browser (browser.nix), which
# draws on it whether or not anyone watches, and the viewer (x11vnc and
# noVNC), which a connection to 6080 starts through systemd-socket-proxyd.
# Starting the viewer starts the browser too, so the desktop is never
# empty. Xvfb and openbox stop by themselves once neither needs them
# (StopWhenUnneeded); nothing runs and nothing is paid for until one of
# them is asked for.
#
# A per-minute check stops the viewer after 30 minutes without a client
# (`systemctl start repose-desktop-idle` stops it now), and the browser
# after 30 minutes with neither a DevTools client (an MCP server holds its
# connection for the agent's whole session) nor a viewer.
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
  # numpy is optional in websockify (it only speeds up unmasking what the
  # browser sends: keys and pointer moves) and costs about 480 MB of
  # closure with openblas, so it is dropped (DECISIONS I-218).
  websockify = pkgs.python312Packages.websockify.overridePythonAttrs (o: {
    dependencies = lib.filter (d: (d.pname or "") != "numpy") o.dependencies;
    dontCheckRuntimeDeps = true;
    doCheck = false;
  });
  # Only the web client's static files: `${pkgs.novnc}` itself carries a
  # novnc_proxy wrapper that pulls in a second Python (3.14) and websockify.
  # defaults.json scales the remote screen to the browser tab; a setting
  # the user changes in noVNC's panel still wins.
  novncWeb = "${pkgs.runCommand "novnc-web" { } ''
    mkdir -p $out
    cp -r ${pkgs.novnc}/share/webapps/novnc/. $out/
    echo '{"resize": "scale"}' > $out/defaults.json
  ''}";
  idleSeconds = 1800;

  # Every window maximised: the browser fills the screen the user watches.
  openboxConfig = pkgs.writeText "repose-openbox-rc.xml" ''
    <?xml version="1.0" encoding="UTF-8"?>
    <openbox_config xmlns="http://openbox.org/3.4/rc">
      <desktops><number>1</number></desktops>
      <applications>
        <application class="*">
          <maximized>yes</maximized>
        </application>
      </applications>
    </openbox_config>
  '';

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
      viewer=false; browser=false
      systemctl is-active --quiet repose-x11vnc.service && viewer=true
      systemctl is-active --quiet repose-browser.service && browser=true
      [ "$viewer" = true ] || [ "$browser" = true ] || exit 0
      now=$(date +%s)
      # age <stamp>: seconds since the stamp, created now when missing.
      age() {
        [ -e "$1" ] || touch "$1"
        echo $((now - $(stat -c %Y "$1")))
      }
      viewers=$(ss -Htn state established '( sport = :6081 or sport = :5900 )' | wc -l)
      devtools=$(ss -Htn state established '( sport = :9225 )' | wc -l)
      [ "$viewers" -eq 0 ] || touch ${dir}/last-client
      [ "$devtools" -eq 0 ] || touch ${dir}/last-cdp
      viewer_idle=$(age ${dir}/last-client)
      cdp_idle=$(age ${dir}/last-cdp)
      if [ "$viewer" = true ] && [ "$viewer_idle" -ge ${toString idleSeconds} ]; then
        systemctl start repose-desktop-idle.service
      fi
      if [ "$browser" = true ] && [ "$viewer_idle" -ge ${toString idleSeconds} ] \
         && [ "$cdp_idle" -ge ${toString idleSeconds} ]; then
        systemctl stop repose-browser-proxy.service repose-browser.service
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
  environment.systemPackages = [ pkgs.xvfb pkgs.openbox pkgs.x11vnc websockify ];

  systemd.services.repose-xvfb = lib.recursiveUpdate common {
    description = "repose desktop: Xvfb ${display}";
    unitConfig.StopWhenUnneeded = true;
    serviceConfig = {
      ExecStart = "${pkgs.xvfb}/bin/Xvfb ${display} -screen 0 1440x900x24 -nolisten tcp -ac -noreset";
      ExecStartPost = "${pkgs.coreutils}/bin/touch ${dir}/last-client";
    };
  };

  systemd.services.repose-openbox = lib.recursiveUpdate common {
    description = "repose desktop: window manager";
    requires = [ "repose-xvfb.service" ];
    after = [ "repose-xvfb.service" ];
    bindsTo = [ "repose-xvfb.service" ];
    unitConfig.StopWhenUnneeded = true;
    serviceConfig.ExecStart = "${pkgs.openbox}/bin/openbox --config-file ${openboxConfig}";
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
    wants = [ "repose-openbox.service" "repose-browser.service" ];
    after = [ "repose-xvfb.service" "repose-openbox.service" ];
    bindsTo = [ "repose-xvfb.service" ];
    serviceConfig = {
      ExecStartPre = "${genPassword}/bin/repose-vnc-password";
      ExecStart = "${pkgs.x11vnc}/bin/x11vnc -display ${display} -localhost -rfbport 5900 -rfbauth ${dir}/vnc-passwd -forever -shared -noxdamage -quiet";
      # A viewer started while the browser had the display up for hours
      # starts its own idle clock.
      ExecStartPost = [ (waitPort 5900) "${pkgs.coreutils}/bin/touch ${dir}/last-client" ];
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

  # Stops the viewer. Xvfb and openbox follow unless the agents' browser
  # still draws on them; the browser has its own idle stop above.
  systemd.services.repose-desktop-idle = {
    description = "repose desktop: stop the viewer";
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${pkgs.systemd}/bin/systemctl stop repose-novnc-proxy.service repose-novnc.service repose-x11vnc.service";
    };
  };

  systemd.services.repose-desktop-idle-check = {
    description = "repose desktop: stop the viewer and the browser after ${toString (idleSeconds / 60)} minutes unused";
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
