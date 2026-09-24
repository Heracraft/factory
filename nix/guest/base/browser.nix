# Chromium, Playwright's browsers, and the two MCP servers every guest has
# (docs/features/browser.md). The agents' browser is one headed Chromium on
# the desktop's X display :99 (DECISIONS I-246): repose-browser.socket on
# 127.0.0.1:9224 starts it, with Xvfb and the window manager, on the first
# DevTools connection, and both MCP servers attach to it there, so
# `repose open --desktop` shows what the agent is doing and the user can
# take over in the same window. Its profile persists in
# ~/.local/share/repose/browser. That browser runs in the system slice
# repose-browser.slice; the MCP servers and any chromium a user starts run
# in the per-user slice of the same name, so a runaway page cannot take the
# agent down with it. The ceiling is a share of the guest's memory, which
# systemd resolves at boot: 37.5 percent is 1.5 GB small, 3 GB large, 6 GB
# xl, so one system closure serves every class (DECISIONS I-34, I-43).
{ config, lib, pkgs, ... }:
let
  browserMemory = "37.5%";
  slice = {
    MemoryMax = browserMemory;
    MemoryHigh = browserMemory;
  };
  display = ":99";
  # The DevTools endpoint the MCP servers use (socket-activated) and the
  # port Chromium itself listens on behind it. Not 9222, which stays free
  # for a Chromium of the user's own; the CLI never auto-forwards either
  # (internal/cli/forward.go).
  cdpPort = 9224;
  backendPort = 9225;
  cdpUrl = "http://127.0.0.1:${toString cdpPort}";
  profileDir = "/home/dev/.local/share/repose/browser";

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
      playwright = { type = "stdio"; command = "playwright-mcp"; args = [ "--cdp-endpoint" cdpUrl ]; };
      chrome-devtools = { type = "stdio"; command = "chrome-devtools-mcp"; args = [ "--browserUrl" cdpUrl ]; };
    };
    # Entries earlier bases registered. repose-agent-setup replaces a
    # user's entry that is exactly one of these, since it wrote them
    # itself, and leaves any other entry alone (user entries win, I-246).
    repose_retired = {
      playwright = [{ type = "stdio"; command = "playwright-mcp"; args = [ "--headless" ]; }];
      chrome-devtools = [{ type = "stdio"; command = "chrome-devtools-mcp"; args = [ "--headless" ]; }];
    };
  };

  # The agents' browser. No --headless: it draws on :99 whether or not
  # anyone watches. The window is the screen's size and the window manager
  # maximises it (desktop.nix). No GPU in a guest: --disable-gpu composites
  # in software instead of relaunching a GPU process that fails to find
  # EGL, and WebGL still works through SwiftShader, as in Playwright's own
  # launches (which pass --enable-unsafe-swiftshader too).
  browserStart = pkgs.writeShellScript "repose-browser-start" ''
    mkdir -p ${profileDir}
    # A kill leaves the profile marked as crashed, and Chromium then offers
    # to restore the last session on top of the agent's page.
    ${pkgs.gnused}/bin/sed -i -e 's/"exit_type":"Crashed"/"exit_type":"Normal"/' \
      -e 's/"exited_cleanly":false/"exited_cleanly":true/' \
      ${profileDir}/Default/Preferences 2>/dev/null || true
    exec ${pkgs.chromium}/bin/chromium \
      --user-data-dir=${profileDir} \
      --remote-debugging-port=${toString backendPort} \
      --no-first-run --no-default-browser-check \
      --password-store=basic \
      --hide-crash-restore-bubble \
      --disable-gpu --enable-unsafe-swiftshader \
      --start-maximized --window-position=0,0 --window-size=1440,900 \
      about:blank
  '';

  # The unit counts as started only once the proxy can connect.
  waitBackend = pkgs.writeShellScript "repose-wait-${toString backendPort}" ''
    for _ in $(seq 1 200); do
      if ${pkgs.iproute2}/bin/ss -Hltn "sport = :${toString backendPort}" | grep -q LISTEN; then exit 0; fi
      sleep 0.1
    done
    echo "port ${toString backendPort} not listening after 20 s" >&2
    exit 1
  '';
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
    sliceConfig = slice;
  };

  systemd.slices.repose-browser = {
    description = "repose: the agents' browser";
    sliceConfig = slice;
  };

  # A crash or an OOM kill of the browser stops this unit and the proxy
  # (BindsTo); the socket keeps listening, the next DevTools connection
  # starts both again with the same profile, and both MCP servers reconnect
  # on their next call.
  systemd.services.repose-browser = {
    description = "repose: the agents' Chromium on ${display}, DevTools on 127.0.0.1:${toString backendPort}";
    requires = [ "repose-xvfb.service" ];
    bindsTo = [ "repose-xvfb.service" ];
    wants = [ "repose-openbox.service" ];
    after = [ "repose-xvfb.service" "repose-openbox.service" ];
    environment = {
      DISPLAY = display;
      HOME = "/home/dev";
      LANG = "C.UTF-8";
    };
    serviceConfig = {
      User = "dev";
      Group = "dev";
      Slice = "repose-browser.slice";
      # TZ, so pages see the project's time zone.
      EnvironmentFile = "-/etc/repose/env";
      ExecStart = browserStart;
      # last-cdp starts the idle clock (desktop.nix).
      ExecStartPost = [ waitBackend "${pkgs.coreutils}/bin/touch /run/repose/desktop/last-cdp" ];
      # A renderer the slice limit kills is one crashed tab, not the end of
      # the browser.
      OOMPolicy = "continue";
      Restart = "no";
      TimeoutStopSec = 10;
      SuccessExitStatus = "0 15 SIGTERM";
    };
  };

  systemd.sockets.repose-browser = {
    description = "repose: the agents' browser DevTools endpoint on 127.0.0.1:${toString cdpPort}";
    wantedBy = [ "sockets.target" ];
    socketConfig = {
      ListenStream = "127.0.0.1:${toString cdpPort}";
      Service = "repose-browser-proxy.service";
    };
  };

  systemd.services.repose-browser-proxy = {
    description = "repose: proxy ${toString cdpPort} to the agents' browser, starting it";
    requires = [ "repose-browser.service" ];
    bindsTo = [ "repose-browser.service" ];
    after = [ "repose-browser.service" ];
    serviceConfig = {
      ExecStart = "${pkgs.systemd}/lib/systemd/systemd-socket-proxyd 127.0.0.1:${toString backendPort}";
      PrivateTmp = true;
      DynamicUser = true;
    };
  };
}
