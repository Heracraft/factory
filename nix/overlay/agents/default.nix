# Platform-owned agent overlay. docs/DECISIONS.md R3-19: agents come from
# here, not nixpkgs, so the platform decides when a tenant's agent changes.
#
# Split of ownership (docs/workstreams/02-guest-base.md §3): workstream 12
# owns the *package definitions* (`reposeAgentsUnwrapped`: each agent from
# its upstream release binary, pinned by version and hash in versions.json,
# bumped by scripts/bump-agents.sh); workstream 02 owns the wrappers, the
# hook plumbing and the MCP servers.
final: prev:
let
  wrap = import ./wrap.nix { pkgs = final; };
in
{
  reposeAgentsUnwrapped = {
    claude-code = final.callPackage ./claude-code.nix { };
    opencode = final.callPackage ./opencode.nix { };
    codex = final.callPackage ./codex.nix { };
    gemini-cli = final.callPackage ./gemini-cli.nix { };
    pi-coding-agent = final.callPackage ./pi-coding-agent.nix { };
  };

  # The five agents, each wrapped per guest-conventions.md "Agent wrappers".
  reposeAgents = builtins.mapAttrs (name: pkg: wrap { inherit name pkg; })
    final.reposeAgentsUnwrapped;

  # Runs once per agent start: idempotent hook and MCP registration.
  repose-agent-setup = final.callPackage ./agent-setup.nix { };

  # Shell implementation of the repose-hook contract (guest-conventions.md);
  # the base uses it until workstream 04's Go binary exists in cmd/repose-hook.
  repose-hook-shim = final.callPackage ./repose-hook.nix { };

  # Playwright's browsers, chromium preset (with the headless shell that
  # headless launches use). The WebKit and Firefox builds are neither wanted
  # nor, at this nixpkgs revision, buildable (WebKit misses libmanette).
  reposePlaywrightBrowsers = prev.playwright-driver.browsers.override {
    withFirefox = false;
    withWebkit = false;
  };

  # nixpkgs's playwright-test bakes the all-browsers set into its wrapper
  # (and playwright-mcp links against it), so its install phase is rewritten
  # to point at the chromium set. The string's other references (the
  # playwright npm build, node) are kept through their contexts; only the
  # all-browsers derivation drops out, so it is never built.
  reposePlaywrightTest =
    let
      old = prev.playwright-test;
      all = prev.playwright-driver.browsers;
      keep = prev.lib.filterAttrs (drv: _: drv != all.drvPath) (builtins.getContext old.installPhase);
      phase = builtins.replaceStrings [ "${all}" ] [ "${final.reposePlaywrightBrowsers}" ]
        (builtins.unsafeDiscardStringContext old.installPhase);
    in old.overrideAttrs (_: { installPhase = builtins.appendContext phase keep; });

  reposeMcp = {
    # nixpkgs keeps playwright-mcp and playwright-driver in step; the two
    # are tightly coupled, so both come from the same locked rev, with the
    # browsers swapped for the chromium preset above.
    playwright-mcp = prev.playwright-mcp.override {
      playwright-test = final.reposePlaywrightTest;
      playwright-driver = prev.playwright-driver // { browsers = final.reposePlaywrightBrowsers; };
    };
    chrome-devtools-mcp = final.callPackage ./chrome-devtools-mcp.nix { };
  };

  reposeOpencodePlugin = ./opencode-plugin.js;
}
