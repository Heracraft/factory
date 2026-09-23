# Headless Chromium, Playwright's browsers, and the two MCP servers every
# guest has (docs/features/browser.md). The MCP servers and chromium enter
# the per-user slice repose-browser.slice so a runaway page cannot take the
# agent down with it. The ceiling is a share of the guest's memory, which
# systemd resolves at boot: 37.5 percent is 1.5 GB small, 3 GB large, 6 GB
# xl, so one system closure serves every class (DECISIONS I-34, I-43).
{ config, lib, pkgs, ... }:
let
  browserMemory = "37.5%";

  # Run a program inside the user's browser slice when a user manager is
  # reachable, else run it directly. `--scope` keeps stdio, which the MCP
  # servers need.
  scope = pkgs.writeShellApplication {
    name = "repose-browser-scope";
    runtimeInputs = [ pkgs.systemd pkgs.coreutils ];
    text = ''
      if [ "$#" -lt 1 ]; then
        echo "usage: repose-browser-scope <program> [args...]" >&2
        exit 64
      fi
      export XDG_RUNTIME_DIR="''${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
      export DBUS_SESSION_BUS_ADDRESS="''${DBUS_SESSION_BUS_ADDRESS:-unix:path=$XDG_RUNTIME_DIR/bus}"
      if [ -z "''${REPOSE_BROWSER_SCOPED:-}" ] \
         && systemd-run --user --quiet --scope --collect --slice=repose-browser.slice -- true 2>/dev/null; then
        export REPOSE_BROWSER_SCOPED=1
        exec systemd-run --user --quiet --scope --collect --slice=repose-browser.slice -- "$@"
      fi
      exec "$@"
    '';
  };

  scoped = name: pkg: bin: pkgs.symlinkJoin {
    name = "repose-${name}";
    paths = [ pkg ];
    postBuild = ''
      rm -f $out/bin/${bin}
      cat > $out/bin/${bin} <<WRAP
      #!${pkgs.runtimeShell}
      exec ${scope}/bin/repose-browser-scope ${pkg}/bin/${bin} "\$@"
      WRAP
      chmod +x $out/bin/${bin}
    '';
    passthru = { unwrapped = pkg; };
    meta = (pkg.meta or { }) // { mainProgram = bin; };
  };

  chromium = scoped "chromium" pkgs.chromium "chromium";
  playwrightMcp = scoped "playwright-mcp" pkgs.reposeMcp.playwright-mcp "playwright-mcp";
  chromeDevtoolsMcp = scoped "chrome-devtools-mcp" pkgs.reposeMcp.chrome-devtools-mcp "chrome-devtools-mcp";

  mcpConfig = {
    mcpServers = {
      playwright = { type = "stdio"; command = "playwright-mcp"; args = [ "--headless" ]; };
      chrome-devtools = { type = "stdio"; command = "chrome-devtools-mcp"; args = [ "--headless" ]; };
    };
  };
in
{
  environment.systemPackages = [
    chromium
    playwrightMcp
    chromeDevtoolsMcp
    scope
  ];

  environment.variables = {
    # PLAYWRIGHT_BROWSERS_PATH is the writable ~/.cache/ms-playwright,
    # seeded with the packaged browsers (compat.nix, DECISIONS I-228); the
    # MCP server's wrapper names the store path itself.
    PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS = "1";
    # chrome-devtools-mcp and puppeteer users: never download a browser.
    PUPPETEER_SKIP_DOWNLOAD = "1";
    PUPPETEER_EXECUTABLE_PATH = "${chromium}/bin/chromium";
    CHROME_BIN = "${chromium}/bin/chromium";
  };

  environment.etc."repose/mcp.json".text = builtins.toJSON mcpConfig;

  fonts = {
    fontconfig.enable = true;
    enableDefaultPackages = false;
    packages = with pkgs; [ noto-fonts noto-fonts-color-emoji liberation_ttf ];
  };

  systemd.user.slices.repose-browser = {
    description = "repose: browsers and browser MCP servers";
    sliceConfig = {
      MemoryMax = browserMemory;
      MemoryHigh = browserMemory;
    };
  };
}
