---
title: Browser
description: Headless Chromium for agents, and a desktop you can open to watch or take over.
section: Using repose
order: 15
---

Every machine has Chromium, and Claude Code on the machine has two MCP servers registered that drive it:

- `playwright` ([Playwright MCP](https://github.com/microsoft/playwright-mcp)): navigate, click, fill forms, take screenshots.
- `chrome-devtools` ([chrome-devtools-mcp](https://github.com/ChromeDevTools/chrome-devtools-mcp)): the same, plus console messages, network requests, performance traces and Lighthouse audits.

Both run headless. Ask the agent to use them:

```
repose run "start the dev server, open the signup page with playwright and screenshot every step of the form"
```

Playwright's browsers are installed already, so scripts and test suites that use Playwright run without `npx playwright install`. Fonts are installed too, so screenshots show real text.

## Watching or taking over: the desktop

Sometimes you need to see the browser, or do something yourself (solve a captcha, sign in with a passkey). The machine has a desktop for that, off until you ask for it.

```
$ repose open --desktop
http://localhost:6080/vnc.html?autoconnect=1 (Ctrl-C stops the forward; the desktop keeps running)
VNC password: 5m2k8Q1p
```

Your browser opens the desktop. Enter the password shown; it changes each time the desktop starts. Press `Ctrl-C` in the terminal to close the connection. The desktop itself keeps running.

New shells on the machine have `DISPLAY=:99` set for as long as the desktop is up, so a browser that an agent starts in headed mode appears on it where you can watch.

The desktop stops by itself after 30 minutes with nobody connected. To stop it now:

```
repose open --desktop --stop
```

> The password line and `--stop` are not in a release yet. With v0.1.9, `repose open --desktop` fails at "start the desktop in the guest"; on the machine, `repose-guest-profile desktop start` starts it and prints the password, then `ssh -L 6080:127.0.0.1:6080 <project>.repose` forwards it.

The desktop is reachable only through your SSH connection. Nothing on the machine accepts connections from the internet.

## Memory limits

A browser page can use a lot of memory. To keep a runaway page from taking the agent down with it, headless Chromium is stopped when it uses more than 1.5 GB on a `small` machine, 3 GB on `large` and 6 GB on `xl`.

## Things that don't work yet

**Claude in Chrome** needs Chrome with the extension installed and signed in to your Claude account, on a real screen. The machine doesn't have that, so it isn't available. The two MCP servers above cover navigation, forms, screenshots, console and network.

**Driving your laptop's own Chrome** from the machine, with your logged-in sessions, is planned as `repose browser bridge`. The command exists but only prints that it isn't available yet.
