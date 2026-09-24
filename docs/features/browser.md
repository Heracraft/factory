# Browser for agents

Every guest has one Chromium that agents drive, headed on a virtual
display, and a desktop viewer that can be switched on when a human needs to
look at or take over that same browser (DECISIONS I-246). Claude in Chrome
is not available in a guest, and the doc says why.

## What the user sees

Agents just use it:

```
$ repose run "log into the staging site and screenshot the dashboard"
```

Watching or taking over:

```
$ repose open --desktop
http://localhost:6080/vnc.html?autoconnect=1 (Ctrl-C stops the forward; the desktop keeps running)
VNC password: 5m2k8Q1p

$ repose open --desktop --stop
Stopped the desktop on todo-app.
```

(The forms `notify set --ntfy` and this output are what shipped, DECISIONS
I-8 and I-241; the desktop also stops itself after 30 minutes with no
client.)

## What is in the guest

- `chromium` from nixpkgs and `playwright-driver.browsers`, linked into
  the usual `~/.cache/ms-playwright` at boot, so a project on the base's
  Playwright version finds its browsers without `npx playwright install`;
  any other version downloads its own there, and the downloaded browsers
  run through nix-ld (DECISIONS I-228).
- The agents' browser, `repose-browser.service`: Chromium, headed, on the
  X display `:99` (Xvfb, 1440x900, with openbox maximising every window),
  its profile in `~/.local/share/repose/browser` so cookies and logins
  survive restarts, DevTools on 127.0.0.1:9225 behind the socket-activated
  endpoint `http://127.0.0.1:9224`. The first connection to 9224 starts
  Xvfb, the window manager and the browser; nothing runs before that.
- Playwright MCP registered in Claude Code's user-scope MCP config as
  `playwright` (`--cdp-endpoint http://127.0.0.1:9224`) and
  chrome-devtools-mcp as `chrome-devtools` (`--browserUrl
  http://127.0.0.1:9224`). Both attach to the agents' browser, so they see
  the same tabs, and Playwright works in its default context: the window
  and the logins the user sees. A guest's earlier `--headless` entries are
  replaced by `repose-agent-setup` at the next agent start; an entry the
  user changed is left alone.
- x11vnc and noVNC as socket-activated systemd units, the viewer. Off until
  asked; they cost nothing idle.
- Fonts (a Liberation and Noto set) so screenshots do not render as boxes.
- The sandbox: Chromium runs as `dev` inside a microVM, so it keeps its own
  sandbox on; nothing needs `--no-sandbox`.

## Behaviour that must hold

- A fresh guest can run a Playwright script that opens a page and takes a
  screenshot with no install step. A test in the guest base does exactly
  that.
- Claude Code in a fresh guest lists `playwright` and `chrome-devtools` in
  `claude mcp list`.
- `repose open --desktop` starts x11vnc bound to localhost and noVNC on
  6080, and with them the agents' browser if it is not running, then
  forwards 6080 over SSH and prints the URL. The user sees the page the
  agent is on, live, and can click and type in it with no prompt and no
  agent restart. noVNC scales the screen to the tab. The VNC password is
  generated per start (read from `/run/repose/desktop/vnc-password` by
  `repose-guest-profile desktop start`) and printed once. Starting when
  already started just forwards. (DECISIONS I-33.)
- While the display is up (the agents' browser or the viewer is running),
  `DISPLAY=:99` is exported into new shells, so a headed browser an agent
  or the user starts appears on the desktop too. Playwright, Puppeteer and
  Cypress default to headless whatever `DISPLAY` says, so a project's test
  suite stays headless unless its config asks otherwise (I-246).
- `repose open --desktop --stop` stops the viewer. The agents' browser
  keeps running for the agent; with it gone, Xvfb stops and `DISPLAY` is
  no longer exported.
- The viewer stops after 30 minutes with no client. The agents' browser
  stops after 30 minutes with no DevTools client (an MCP server keeps its
  connection for the agent's whole session) and no viewer client.
- A crashed or killed browser starts again on the next DevTools
  connection with the same profile, and both MCP servers reconnect on
  their next call.
- The desktop is never reachable except through the SSH forward. noVNC
  binds `127.0.0.1` in the guest; the guest has no inbound anyway.
- The browser is limited to 1.5 GB on a small guest, 3 GB on large, 6 GB
  on xl (the slice `repose-browser.slice`, system level for the agents'
  browser, user level for the MCP servers and any Chromium a user starts),
  because a runaway page in a 4 GB guest takes the agent down with it and
  the user only sees an unexplained stall. A renderer killed at the limit
  is one crashed tab; the browser stays up.
- The DevTools ports 9224 and 9225 are never auto-forwarded to the laptop.

## Why Claude in Chrome cannot work here

Claude in Chrome needs a visible Chrome with the extension installed, a
subscription login, and connects through Anthropic's relay from that
browser. The guest has no such Chrome and the extension cannot be bridged
from the laptop today. Agents that need browsing use Playwright MCP or
chrome-devtools-mcp instead, which cover navigation, forms, screenshots,
console and network capture.

## Planned: `repose browser bridge`

For the "open Chrome and go to my thing" case while the laptop is open:

1. The CLI starts or finds the laptop's Chrome with a DevTools port
   (`--remote-debugging-port`) on localhost.
2. It opens an SSH reverse tunnel from a guest port to that port.
3. It rewrites the guest's `playwright` and `chrome-devtools` MCP entries to
   attach to `http://127.0.0.1:<port>` (`--cdp-endpoint`, `--browserUrl`)
   for the life of the CLI process, restoring the guest-browser entries on
   exit.

The agent then drives the laptop's real browser, with the user's sessions
and extensions. It works only while the laptop is open and the tunnel is
up; the doc says so and the CLI says so.

## Depends on

Workstreams 02 (packages, MCP registration, units, slice limits), 07 (`open
--desktop`, port forward), 04 (start/stop units, DISPLAY export via guestd
Exec).

## Deferred

`repose browser bridge`. Browserbase or another hosted browser as an
option. GPU-accelerated rendering (no GPU guests).
