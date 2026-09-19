# Platform-owned agent overlay. docs/DECISIONS.md R3-19: agents come from
# here, not nixpkgs, so the platform decides when a tenant's agent changes.
#
# Split of ownership (docs/workstreams/02-guest-base.md §3): workstream 12
# owns the *package definitions* (`reposeAgentsUnwrapped`, to be replaced by
# builds from versions.json and bumped by scripts/bump-agents.sh); this
# workstream owns the wrappers, the hook plumbing and the MCP servers.
# Until 12 lands, each unwrapped agent resolves to nixpkgs at the flake's
# locked revision, which is a pin, not a moving target.
final: prev:
let
  wrap = import ./wrap.nix { pkgs = final; };
in
{
  reposeAgentsUnwrapped = {
    claude-code = prev.claude-code;
    opencode = prev.opencode;
    codex = prev.codex;
    gemini-cli = prev.gemini-cli;
    pi-coding-agent = prev.pi-coding-agent;
  };

  # The five agents, each wrapped per guest-conventions.md "Agent wrappers".
  reposeAgents = builtins.mapAttrs (name: pkg: wrap { inherit name pkg; })
    final.reposeAgentsUnwrapped;

  # Runs once per agent start: idempotent hook and MCP registration.
  repose-agent-setup = final.callPackage ./agent-setup.nix { };

  # Shell implementation of the repose-hook contract (guest-conventions.md);
  # the base uses it until workstream 04's Go binary exists in cmd/repose-hook.
  repose-hook-shim = final.callPackage ./repose-hook.nix { };

  reposeMcp = {
    # nixpkgs keeps playwright-mcp and playwright-driver.browsers in step;
    # the two are tightly coupled, so both come from the same locked rev.
    playwright-mcp = prev.playwright-mcp;
    chrome-devtools-mcp = final.callPackage ./chrome-devtools-mcp.nix { };
  };

  reposeOpencodePlugin = ./opencode-plugin.js;
}
