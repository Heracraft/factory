---
title: Troubleshooting
description: The errors you're most likely to see and what fixes each one.
section: Reference
order: 41
---

For more detail on any command, add `-v`. `REPOSE_TIMING=1` shows where the time in `run` and `attach` went, and `repose logs --kind ops` lists every operation on the project with its result.

## Logging in and connecting

**``Not logged in. Run `repose login`.``** Your login expired or you logged out. Run `repose login`.

**`No repose project for github.com/you/app`** This checkout has no project yet, or its remote changed. `repose run` creates one; `repose attach NAME` reaches an existing one.

**`This directory has no git remote.`** Give the project a name: `repose run --name scratch`.

**`ssh todo-app.repose` says `Permission denied`.** Your certificate is older than 24 hours. Run `repose attach todo-app`, detach, and try again. If the CLI works but plain `ssh` never does, the `Include ~/.ssh/repose/config` line is missing from `~/.ssh/config`; the CLI printed a message when it couldn't add it.

**`Guest is running but SSH did not answer in 60s.`** `repose logs --kind console` shows the boot log. `repose start` restarts a stuck machine.

## Machine state

**``todo-app is stopped. Start it with `repose start todo-app` ...``** `attach`, `open` and `cp` don't start machines. Run `repose start todo-app`, or `repose run` in the checkout.

**`todo-app is in an error state`** or **`guestd stopped answering`.** `repose start todo-app` restarts it. If that fails, `repose logs todo-app --kind console` shows what happened during boot.

**`No capacity right now`.** The servers are full. Nothing was changed. Try again in a few minutes.

## Sync

**`The machine has uncommitted changes your laptop doesn't have`.** Something on the machine, usually an agent, changed files since your last sync, and your laptop has new work that would write over them. `repose attach` to look, or re-run with `--stash-remote` to keep them in `git stash` or `--discard-remote` to drop them. With nothing new on your laptop, `repose run` just attaches. See [Sync](/docs/sync#when-the-machine-has-changes-of-its-own).

**`Not sent: web/node_modules`.** Dependency directories never travel. Run your install command on the machine.

**A big file wasn't sent.** Files over 100 MB and untracked files past 500 MB per sync are skipped. Commit what matters, or add it to `.gitignore`.

## On the machine

**A command isn't found.** The machine prints the nixpkgs package that has it and the two ways to add it. If it says the tool `is still being installed`, it's one of your laptop's tools arriving in the background; try again shortly.

**A tool from your laptop didn't arrive.** The next `repose run` names it. The log is `~/.repose/tools-install.log` on the machine. `repose scan` shows what the CLI looked for.

**A program you installed isn't on `PATH`.** Installs with npm, pnpm, `go install`, `cargo install`, uv, pip `--user`, bun, deno, gem and composer are on `PATH` in new shells. Open a new tmux window. Tools that manage `PATH` from their own shell setup (nvm, pyenv, rbenv) need that setup in `~/.bashrc`.

**Processes get killed, or the machine is slow under load.** It ran out of memory: `sudo dmesg | grep -i killed` names what the kernel stopped. Stop what you don't need (`repose status` lists dev servers still listening), or give the machine more memory with `repose resize --size large` or `--size xl`, which restarts it. See [Changing the size](/docs/machine#changing-the-size).

**A secret isn't in a program's environment.** Programs read their environment when they start. Open a new tmux window, or restart the program or agent.

**`config error` or exit code 10.** The build failed and nothing changed. The message names the problem; `repose logs --kind build` has the full log. Fix it with `repose config edit` or `repose config remove`.

## Agents

**`Claude Code is not logged in on this guest yet.`** Finish the login in the window the CLI opened, then run your prompt again. See [Agents](/docs/agents#log-in).

**An agent seems stuck.** Attach and look; it's usually waiting on a permission prompt. See [Let it run without asking](/docs/agents#let-it-run-without-asking).

## Ports

**My dev server isn't on localhost.** Check that:

- you're attached (`repose run` or `repose attach`);
- the server listens on `127.0.0.1`, `localhost` or `0.0.0.0`, on port 1024 or higher;
- `REPOSE_NO_FORWARD` isn't set;
- it didn't move to another port because yours was taken (tmux shows the new one).

`repose open PORT` forwards one port by hand.

## Notifications

**Nothing arrives.** Run `repose notify test`. An `error` means that channel's settings are wrong. If both are `ok`, `repose events` shows whether the event happened; a project sends at most 30 notifications an hour.
