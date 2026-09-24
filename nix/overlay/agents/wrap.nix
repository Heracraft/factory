# Wrap one agent: same package, same binary name, but the entry point sets
# the terminal environment, runs the idempotent hook registration for that
# agent, and execs the real binary with "$@". The binary is never modified
# (docs/features/agents.md "Claude login and the policy behind it").
{ pkgs }:
{ name, pkg }:
let
  bin = pkg.meta.mainProgram or name;
  wrapper = pkgs.writeShellScript "repose-${bin}-wrapper" ''
    if [ -n "''${TMUX:-}" ]; then
      export TERM=tmux-256color
    fi
    export COLORTERM=truecolor
    # `repose run "prompt"` starts the agent as tmux new-window's command,
    # a shell that is neither login nor interactive, so /etc/profile never
    # ran: without this the agent has no named secrets
    # (CLAUDE_CODE_OAUTH_TOKEN, GEMINI_API_KEY), no REPOSE_PROJECT and no
    # ~/.local/bin on PATH. The file is idempotent (env.nix).
    if [ -r /etc/profile.d/repose.sh ]; then
      . /etc/profile.d/repose.sh
    fi
    export REPOSE_HOOK_AGENT=${bin}
    # Registration failures must never block an agent (a blocked agent is a
    # silently wasted night), so setup is best-effort.
    ${pkgs.repose-agent-setup}/bin/repose-agent-setup ${bin} || true
    exec ${pkg}/bin/${bin} "$@"
  '';
in
pkgs.symlinkJoin {
  name = "repose-${name}-${pkg.version or "0"}";
  paths = [ pkg ];
  postBuild = ''
    rm -f $out/bin/${bin}
    cp ${wrapper} $out/bin/${bin}
    chmod +x $out/bin/${bin}
  '';
  passthru = {
    unwrapped = pkg;
    binary = bin;
    inherit (pkg) version;
  };
  # The wrapper itself is free; the licence check ran on `pkg` already.
  meta = (builtins.removeAttrs (pkg.meta or { }) [ "license" ]) // { mainProgram = bin; };
}
