# repose-guest-profile: the small script the CLI's `open`, `sync` and the
# hooks rely on (DESIGN.md §5). It answers "what is this guest" in JSON and
# drives the on-demand desktop, so the CLI never has to know unit names.
#   repose-guest-profile                 -> JSON (see guest-conventions.md)
#   repose-guest-profile desktop start   -> starts the chain, prints the
#                                           noVNC password
#   repose-guest-profile desktop stop
#   repose-guest-profile desktop status  -> running|stopped
{ config, lib, pkgs, ... }:
let
  script = pkgs.writeShellApplication {
    name = "repose-guest-profile";
    runtimeInputs = [ pkgs.jq pkgs.coreutils pkgs.systemd ];
    text = ''
      project=/home/dev/.repose/project.json
      # The desktop is the viewer; the display alone may be up for the
      # agents' browser (DECISIONS I-246).
      desktop_running() {
        systemctl is-active --quiet repose-x11vnc.service
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
              # the agents' browser included.
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
        *)
          echo "usage: repose-guest-profile [desktop start|stop|status]" >&2
          exit 64
          ;;
      esac
    '';
  };
in
{
  environment.systemPackages = [ script ];
}
