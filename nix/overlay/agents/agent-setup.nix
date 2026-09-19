# repose-agent-setup <agent>: make the agent report to repose-hook.
# Idempotent, never clobbers user configuration (the VM test asserts a
# pre-existing hook survives), exits 0 on every path so a wrapper can `|| true`.
#
# claude   ~/.claude/settings.json  hooks.Notification / hooks.Stop entries
#          running `repose-hook` are added unless an entry whose command
#          contains "repose-hook" already exists under that event;
#          ~/.claude.json mcpServers gains the platform servers from
#          /etc/repose/mcp.json, user entries winning on name clash.
# codex    ~/.codex/config.toml gains `notify = ["repose-hook"]` unless a
#          `notify` key already exists.
# opencode ~/.config/opencode/plugins/repose.js is installed if absent.
# gemini, pi: nothing; guestd's pane-idle heuristic reports for them.
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
          jq -s '.[0] as $u | .[1] as $p | $u | .mcpServers = ($p.mcpServers + ($u.mcpServers // {}))' \
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
      gemini|pi) ;;
      *) echo "repose-agent-setup: unknown agent '$agent'" >&2 ;;
    esac
    exit 0
  '';
}
