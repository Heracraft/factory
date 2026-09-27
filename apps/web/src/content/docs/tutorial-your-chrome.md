---
title: Let the agent use your Chrome
description: Lend the agents on a machine your laptop's Chrome, logins and extensions included, for as long as you keep the bridge open.
section: Tutorials
order: 22
---

Some jobs need the browser you already have: the admin tool behind your company's SSO, an account protected by a hardware key, a site where an extension does half the work. Logging in to all of that on the machine is the wrong answer. `repose browser bridge` is the right one: while it runs, the machine's browser tools drive your laptop's Chrome instead of the machine's, and nothing else changes.

## Once: turn Chrome's switch on

Chrome 144 and newer can accept a debugging connection to the profile you're using. Open this in Chrome and turn it on:

```
chrome://inspect/#remote-debugging
```

That's the only setup. It stays on until you turn it off. (If you skip this step, the bridge command opens the page for you and waits.)

## Open the bridge

From a terminal on your laptop, in the project's checkout or with the project's name:

```
$ repose browser bridge
Chrome 144 → todo-app: the agents there browse in your Chrome now, with your logins. Ctrl-C hands them back the machine's browser.
Chrome asks you to allow each new connection.
```

Leave that terminal open. Now give the agent the job in another one:

```
repose run "open the admin dashboard at https://admin.internal.example, export last week's signups as CSV and put the file in data/"
```

The first time the agent's browser tool connects, Chrome shows a dialog asking whether to allow the connection. Allow it. The bridge terminal says:

```text
An agent on todo-app is in your Chrome.
```

and a new tab appears in your Chrome with the dashboard, logged in as you, driven by the agent. Chrome shows its "being controlled by automated test software" bar while a connection is open. You can watch, and you can use your other tabs as usual.

## Close it

`Ctrl-C` in the bridge terminal:

```text
Bridge closed. The agents on todo-app are back on the machine's browser.
```

The agent's next browser call lands in the machine's own browser again, with nothing restarted. The tab the agent opened stays in your Chrome for you to close.

## Bridge for the whole session

If you'd rather not keep a second terminal, `--bridge` on `run` or `attach` keeps the bridge open for as long as you're attached:

```
repose run --bridge "check the staging site's checkout flow in my browser and fix what breaks"
```

A message in tmux tells you when the bridge is up, and Chrome still asks you to allow each connection. Detach, and the bridge closes with the attach.

## What you're lending

Be clear about this before you open a bridge on a shared machine:

- **Everything on the machine can use your Chrome** while the bridge is open, not only the agent you're watching. That is what the machine's browser tools are: a socket every process there can reach.
- **Every site you're logged in to** is reachable in that Chrome. Chrome's per-connection dialog is your check on who connects; the bridge's terminal tells you when a connection arrives.
- **It lasts while your laptop is awake and connected.** Close the laptop and the machine's tools go back to its own browser within two minutes. For work that should run overnight, the machine's own browser is the one to log in to, through [the desktop](/docs/tutorial-watch-browser).

`Ctrl-C` as soon as the job is done.

## Another browser, or an older Chrome

Any Chromium browser started with a remote debugging port can be bridged as is, with no switch and no dialogs:

```
chromium --remote-debugging-port=9222 --user-data-dir=$HOME/agent-chrome
repose browser bridge --cdp http://127.0.0.1:9222
```

That gives the agent a browser of its own on your laptop, with a separate profile you log in to once. Brave, Edge and Chromium profiles that aren't Google Chrome's default one work through `--user-data-dir DIR` when their switch is on.

[The machine](/docs/machine#use-your-own-chrome) and the [CLI reference](/docs/cli#repose-browser-bridge-project) have the details.
