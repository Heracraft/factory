---
title: Quickstart
description: From nothing to an agent working on your repository.
section: Start here
order: 2
---

You need a Mac or a Linux machine with `git` and the OpenSSH client (`ssh`), a GitHub account, and a git checkout with a remote. On Windows, run these steps inside WSL.

## 1. Install the CLI

```
curl -fsSL https://repose.herakraft.co/install.sh | sh
```

This puts `repose` in `~/.local/bin`. If that directory isn't on your `PATH`, the script adds it to your shell's startup file and says so; open a new terminal before going on. [Install](/docs/install) has the other options.

## 2. Log in

```
repose login
```

The CLI prints a URL and a code:

```
Open https://accounts.herakraft.co/... and enter code ABCD-EFGH
Waiting...
```

Open the URL on any device, sign in with GitHub and enter the code. The terminal finishes with:

```
Logged in as yourhandle (you@example.com)
```

## 3. Add a card

A machine won't start until your account has a card on file. Go to [Billing](https://repose.herakraft.co/billing) in the dashboard and choose **Add a card**. The card form is Stripe's own page.

Your first day of compute is free. That's one day on a `large` machine, or two on a `small` one. The credit is used before anything is charged to the card.

## 4. Run it in a checkout

```
cd ~/code/your-project
repose run
```

The first run creates the project, builds its environment and boots it. That takes under a minute. You'll see something like:

```
✓ Created your-project (large)  0.3s
✓ Built the environment  41s
✓ Booted your-project  6.2s
Connected to your-project (large)
Synced: 3 modified, 1 untracked (2 new commits)
Credentials: gh, git
Ready in 49s.
```

You're now in a tmux session on the machine, in `/home/dev/your-project`, with your uncommitted changes applied. Look around. Run your tests. It's a normal Linux shell.

## 5. Log in to Claude Code on the machine

Claude Code's login is never copied from your laptop, so you log in once on each machine. Type `claude` in the tmux session. It prints a URL. Open it on your laptop, approve, and paste the code back into the terminal. The login stays on the machine's disk for as long as the project exists.

If you use Codex, opencode or the GitHub CLI on your laptop, those logins were already copied over in step 4.

## 6. Give an agent a job

Detach first with `Ctrl-b` then `d`. You're back on your laptop. Now:

```
repose run "write tests for src/billing.ts, run them, commit when they pass"
```

The CLI starts Claude Code in a new tmux window on the machine, waits for it to be ready, types your prompt and attaches you to that window. Watch for a while, or detach. The agent keeps working either way.

## 7. Get told when it's done

```
repose notify set --ntfy https://ntfy.sh/pick-a-long-random-topic-name
repose notify test
```

Install the [ntfy app](https://ntfy.sh) on your phone and subscribe to the same topic. Email notifications are on by default and go to the address shown on the dashboard's Account page. [Notifications](/docs/notifications) has the details.

## 8. Come back

```
repose attach your-project
```

This works from the checkout with no argument, or from anywhere with the project name. Once the agent has pushed its work, pull it on your laptop as usual.

## 9. Stop it when you're done for the day

```
repose stop
```

A stopped machine costs only its disk. The next `repose run` starts it again in about 15 seconds, with your files and installed packages where you left them. Running processes, including agents, don't survive a stop.
