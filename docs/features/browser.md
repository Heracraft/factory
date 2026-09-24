# Browser for agents

Every guest has a headless Chromium that agents can drive, and a desktop
that can be switched on when a human needs to look at or take over that
browser. Claude in Chrome is not available in a guest, and the doc says why.

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
I-8 and I-227; the desktop also stops itself after 30 minutes with no
client.)

## What is in the guest

- `chromium` from nixpkgs and `playwright-driver.browsers` so Playwright
  finds its browsers without `npx playwright install`, which would fail on
  NixOS and pull a second copy anyway.
- Playwright MCP registered in Claude Code's user-scope MCP config as
  `playwright`, headless by default. chrome-devtools-mcp registered as
  `chrome-devtools`, also headless. Both can attach to an already running
  Chrome over the DevTools protocol, which is what the desktop mode and the
  later laptop bridge rely on.
- Xvfb, a minimal window manager, x11vnc and noVNC as socket-activated
  systemd units. Off until asked; they cost nothing idle.
- Fonts (a Liberation and Noto set) so screenshots do not render as boxes.
- The sandbox: Chromium runs as `dev` inside a microVM, so it keeps its own
  sandbox on; nothing needs `--no-sandbox`.

## Behaviour that must hold

- A fresh guest can run a Playwright script that opens a page and takes a
  screenshot with no install step. A test in the guest base does exactly
  that.
- Claude Code in a fresh guest lists `playwright` and `chrome-devtools` in
  `claude mcp list`.
- `repose open --desktop` starts Xvfb on `:99`, the window manager, x11vnc
  bound to localhost, and noVNC on 6080, then forwards 6080 over SSH and
  prints the URL. The VNC password is generated per start (read from
  `/run/repose/desktop/vnc-password` by `repose-guest-profile desktop
  start`) and printed once. Starting when already started just forwards.
  (DECISIONS I-33.)
- When the desktop is up, `DISPLAY=:99` is exported into new shells in the
  tmux session, so an agent asked to "open a headed browser" gets one on the
  desktop and the user can see it in noVNC.
- `repose open --desktop --stop` stops the units and clears `DISPLAY`.
- The desktop is never reachable except through the SSH forward. noVNC
  binds `127.0.0.1` in the guest; the guest has no inbound anyway.
- Headless Chromium is killed when its RSS passes 1.5 GB on a small guest,
  3 GB on large, 6 GB on xl (a systemd slice limit), because a runaway
  page in a 4 GB guest takes the agent down with it and the user only sees
  an unexplained stall.

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
   attach to `http://127.0.0.1:<port>` (`--cdp-endpoint`, `--browser-url`)
   for the life of the CLI process, restoring headless entries on exit.

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
