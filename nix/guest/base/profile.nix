# repose-guest-profile: the small script the CLI's `open`, `sync` and the
# hooks rely on (DESIGN.md §5). It answers "what is this guest" in JSON and
# drives the on-demand desktop, so the CLI never has to know unit names.
#   repose-guest-profile                 -> JSON (see guest-conventions.md)
#   repose-guest-profile desktop start   -> starts the chain, prints the
#                                           viewer's password (one per boot)
#   repose-guest-profile desktop stop
#   repose-guest-profile desktop status  -> running|stopped
#   repose-guest-profile browser bridge start|stop|status|release
#                                        -> the DevTools endpoint agents use
#                                           points at the laptop's Chrome
#                                           (start) or the machine's (stop);
#                                           status prints on|off; release
#                                           ends a previous bridge's ssh
#   repose-guest-profile browser bridge hold
#                                        -> start, then keep the bridge on
#                                           until stdin closes or the tunnel
#                                           is gone, then stop (the CLI runs
#                                           this as the remote command of the
#                                           ssh that carries the tunnel, so
#                                           the bridge lives exactly as long
#                                           as that ssh, DECISIONS I-296)
{ config, lib, pkgs, ... }:
let
  script = pkgs.writeShellApplication {
    name = "repose-guest-profile";
    runtimeInputs = [ pkgs.jq pkgs.coreutils pkgs.systemd ];
    text = ''
      project=/home/dev/.repose/project.json
      # The desktop is the viewer (websockify and the page); the display,
      # Xvnc, may be up for the agents' browser alone (DECISIONS I-246,
      # I-292).
      desktop_running() {
        systemctl is-active --quiet repose-novnc.service
      }
      case "''${1:-}" in
        "")
          base=$(cat /etc/repose/base-version 2>/dev/null || echo unknown)
          if desktop_running; then running=true; else running=false; fi
          if [ -s "$project" ]; then
            proj=$(cat "$project")
          else
            proj='{}'
          fi
          jq -n --argjson project "$proj" --arg base "$base" --argjson running "$running" \
            '{
              project_id: ($project.project_id // null),
              slug: ($project.slug // null),
              name: ($project.name // null),
              dir: (if $project.slug then "/home/dev/" + $project.slug else null end),
              tz: ($project.tz // null),
              class: ($project.class // null),
              base_version: $base,
              desktop: { running: $running, display: ":99", novnc_port: 6080,
                         password_file: "/run/repose/desktop/vnc-password" }
            }'
          ;;
        desktop)
          case "''${2:-}" in
            start)
              # Starting the socket's service pulls the whole chain in,
              # the agents' browser included. The password is the boot's
              # (desktop.nix), the same at every start until a reboot.
              sudo -n systemctl start repose-novnc.service
              cat /run/repose/desktop/vnc-password
              ;;
            stop)
              sudo -n systemctl start repose-desktop-idle.service
              ;;
            status)
              if desktop_running; then echo running; else echo stopped; fi
              ;;
            *)
              echo "usage: repose-guest-profile desktop start|stop|status" >&2
              exit 64
              ;;
          esac
          ;;
        browser)
          case "''${2:-} ''${3:-}" in
            "bridge start") sudo -n repose-browser-bridge on ;;
            "bridge stop") sudo -n repose-browser-bridge off ;;
            "bridge status") repose-browser-bridge status ;;
            "bridge release") repose-browser-bridge release ;;
            "bridge hold")
              sudo -n repose-browser-bridge on
              # Whatever ends this (Ctrl-C on the laptop, a connection
              # sshd gave up on, the tunnel listener gone), the endpoint
              # goes back to the machine's browser.
              trap 'sudo -n repose-browser-bridge off' EXIT
              echo on
              while :; do
                # read: 0 for a line, over 128 on the timeout, 1 at EOF,
                # which is the ssh session ending.
                read -r -t 5 _ && continue
                rc=$?
                [ "$rc" -gt 128 ] || break
                repose-browser-bridge tunnel || break
              done
              ;;
            *)
              echo "usage: repose-guest-profile browser bridge start|stop|status|release|hold" >&2
              exit 64
              ;;
          esac
          ;;
        *)
          echo "usage: repose-guest-profile [desktop start|stop|status] [browser bridge start|stop|status|release|hold]" >&2
          exit 64
          ;;
      esac
    '';
  };
in
{
  environment.systemPackages = [ script ];
}
