# repose-agent-setup <agent>: make the agent report to repose-hook.
# Idempotent, never clobbers user configuration (the VM test asserts a
# pre-existing hook survives), exits 0 on every path so a wrapper can `|| true`.
#
# claude   ~/.claude/settings.json  hooks.Notification / hooks.Stop entries
#          running `repose-hook` are added unless an entry whose command
#          contains "repose-hook" already exists under that event;
#          ~/.claude.json mcpServers gains the platform servers from
#          /etc/repose/mcp.json, user entries winning on name clash
#          except an entry the platform registered itself in an earlier
#          base (mcp.json's repose_retired), which is replaced.
# codex    ~/.codex/config.toml gains `notify = ["repose-hook"]` unless a
#          `notify` key already exists.
# opencode ~/.config/opencode/plugins/repose.js is installed if absent.
# gemini, pi: no hooks (guestd's pane-idle heuristic reports for them); the
#          machine guide (DECISIONS I-243) is linked in as an extension:
#          ~/.gemini/extensions/repose-machine-guide -> /etc/repose/gemini-extension,
#          ~/.pi/agent/extensions/repose-machine-guide.js -> /etc/repose/pi-extension.js.
#          A file or directory the user put at either path is left alone.
{ lib, writeShellApplication, jq, coreutils, reposeOpencodePlugin }:
writeShellApplication {
  name = "repose-agent-setup";
  runtimeInputs = [ jq coreutils ];
  text = ''
    agent="''${1:-}"
    platform_claude=/etc/repose/claude-settings.json
    platform_mcp=/etc/repose/mcp.json

    write_atomic() { # write_atomic <path> <mode>  (content on stdin)
      local path="$1" mode="$2" tmp
      tmp=$(mktemp -p "$(dirname "$path")")
      cat > "$tmp"
      chmod "$mode" "$tmp"
      mv -f "$tmp" "$path"
    }

    setup_claude() {
      local settings="$HOME/.claude/settings.json" userjson="$HOME/.claude.json"
      mkdir -p "$HOME/.claude"
      if [ -r "$platform_claude" ]; then
        if [ ! -s "$settings" ]; then
          write_atomic "$settings" 0600 < "$platform_claude"
        elif jq -e . "$settings" >/dev/null 2>&1; then
          jq -s '
            def has_repose(arr): ((arr // []) | any(.[]; ((.hooks // []) | any(.[]; ((.command // "") | contains("repose-hook"))))));
            .[0] as $user | .[1] as $platform
            | reduce ($platform.hooks | keys[]) as $ev ($user;
                if has_repose(.hooks[$ev]) then .
                else .hooks[$ev] = ((.hooks[$ev] // []) + $platform.hooks[$ev]) end)
          ' "$settings" "$platform_claude" | write_atomic "$settings" 0600
        else
          echo "repose-agent-setup: $settings is not valid JSON; leaving it alone" >&2
        fi
      fi
      if [ -r "$platform_mcp" ]; then
        if [ ! -s "$userjson" ]; then
          jq '{ mcpServers: .mcpServers }' "$platform_mcp" | write_atomic "$userjson" 0600
        elif jq -e . "$userjson" >/dev/null 2>&1; then
          # A user entry that is exactly one the platform registered
          # before (repose_retired) was written here, not by the user, and
          # gives way to the current one (I-246).
          jq -s '.[0] as $u | .[1] as $p
            | (($u.mcpServers // {}) | with_entries(
                .key as $k | .value as $v
                | select(any((($p.repose_retired // {})[$k] // [])[]; . == $v) | not))) as $kept
            | $u | .mcpServers = ($p.mcpServers + $kept)' \
            "$userjson" "$platform_mcp" | write_atomic "$userjson" 0600
        else
          echo "repose-agent-setup: $userjson is not valid JSON; leaving it alone" >&2
        fi
      fi
    }

    setup_codex() {
      local cfg="$HOME/.codex/config.toml"
      mkdir -p "$HOME/.codex"
      if [ ! -e "$cfg" ]; then
        printf 'notify = ["repose-hook"]\n' | write_atomic "$cfg" 0600
      elif ! grep -Eq '^[[:space:]]*notify[[:space:]]*=' "$cfg"; then
        { printf 'notify = ["repose-hook"]\n'; cat "$cfg"; } | write_atomic "$cfg" 0600
      fi
    }

    # link_owned <link> <target>: the link is ours by name; point it at the
    # target unless something that is not a symlink sits there.
    link_owned() {
      local link="$1" target="$2"
      [ -e "$target" ] || return 0
      if [ -L "$link" ]; then
        [ "$(readlink "$link")" = "$target" ] || ln -sfn "$target" "$link"
      elif [ -e "$link" ]; then
        echo "repose-agent-setup: $link is not a link to $target; leaving it alone" >&2
      else
        mkdir -p "$(dirname "$link")"
        ln -s "$target" "$link"
      fi
    }

    setup_gemini() {
      link_owned "$HOME/.gemini/extensions/repose-machine-guide" /etc/repose/gemini-extension
    }

    setup_pi() {
      link_owned "''${PI_CODING_AGENT_DIR:-$HOME/.pi/agent}/extensions/repose-machine-guide.js" /etc/repose/pi-extension.js
    }

    setup_opencode() {
      local dir="$HOME/.config/opencode/plugins"
      mkdir -p "$dir"
      if [ ! -e "$dir/repose.js" ]; then
        write_atomic "$dir/repose.js" 0644 < ${reposeOpencodePlugin}
      fi
    }

    case "$agent" in
      claude) setup_claude ;;
      codex) setup_codex ;;
      opencode) setup_opencode ;;
      gemini) setup_gemini ;;
      pi) setup_pi ;;
      *) echo "repose-agent-setup: unknown agent '$agent'" >&2 ;;
    esac
    exit 0
  '';
}
