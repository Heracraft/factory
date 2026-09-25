---
title: Quickstart
description: Install the CLI, start a machine for your project and hand a task to an agent.
section: Start here
order: 1
---

repose gives each of your projects its own Linux machine in the cloud, with your code, tools and logins already on it. An agent there can run with full permissions for as long as the work takes. The worst it can do is wreck that one machine, and a snapshot puts it back. Your laptop, your SSH keys and your other projects are out of its reach.

You need macOS or Linux (on Windows, use WSL) with `git` and `ssh`, a GitHub account, and a git checkout.

## 1. Install and log in

```
curl -fsSL https://repose.herakraft.co/install.sh | sh
repose login
```

`repose login` prints a URL and a code. Open the URL on any device, sign in with GitHub and enter the code. [Install](/docs/install) has the other ways to install.

## 2. Start a machine for your checkout

```
cd ~/code/your-project
repose run
```

```
✓ Created your-project (large)  0.3s
✓ Built the environment  6.1s
✓ Booted your-project  5.2s
Connected to your-project (large)
Synced: 3 modified, 1 untracked (2 new commits)
Credentials: gh
Ready in 14s.
```

You're now in a shell on the machine, in `/home/dev/your-project`, with your uncommitted changes and unpushed commits applied. The shell runs inside tmux, a terminal session that keeps running when you disconnect.

## 3. Log in to Claude Code on the machine

Claude Code's login is never copied from your laptop, so you log in once per machine. Type `claude`, open the URL it prints, approve, and paste the code back. The login stays on the machine's disk.

Codex, opencode and GitHub CLI logins were copied from your laptop in step 2. [Agents](/docs/agents) covers the rest.

## 4. Let it run without asking

Claude Code on the machine starts in `bypassPermissions` mode, so it doesn't stop to ask before running a command. If your laptop's `~/.claude/settings.json` sets another `defaultMode`, the machine uses yours. [Agents](/docs/agents#let-it-run-without-asking) has the details.

## 5. Hand it a task

Detach from tmux with `Ctrl-b` then `d`. Back on your laptop:

```
repose run "write tests for src/billing.ts, run them, commit and push when they pass"
```

The CLI starts Claude Code in a new tmux window on the machine, types your prompt and attaches you. Watch, or detach and close the laptop. The agent keeps working.

## 6. Get a notification when it's done

Email is on by default. For your phone, pick a long random [ntfy](https://ntfy.sh) topic, subscribe to it in the ntfy app, then:

```
repose notify set --ntfy https://ntfy.sh/repose-4f9c2a7e1b3d5c8a0f6e
repose notify test
```

## 7. Come back and stop

From any computer you're logged in on:

```
repose attach your-project
```

When the agent has pushed, pull on your laptop as usual. Nothing syncs back on its own. Stop the machine when you're done:

```
repose stop
```

A stopped machine costs only its disk. The next `repose run` starts it again in about 10 seconds with your files where you left them. Running processes, agents included, don't survive a stop.

## Next

- [Run and attach](/docs/run-and-attach): tmux, agents, SSH and editors.
- [Sync](/docs/sync): what travels to the machine and what doesn't.
- [The machine](/docs/machine): what's installed, ports, the browser.
- [Pricing](/docs/billing).
