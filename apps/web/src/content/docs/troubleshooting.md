---
title: Troubleshooting
description: Exit codes, the errors you're most likely to see, and what fixes each one.
section: Reference
order: 40
---

## Exit codes

Every `repose` command uses the same exit codes, so scripts can act on them.

| Code | Meaning                                                      | Usual fix                                                       |
| ---- | ------------------------------------------------------------ | --------------------------------------------------------------- |
| 0    | Worked.                                                      |                                                                 |
| 1    | Something failed; the message says what.                     | Read the message.                                               |
| 2    | Wrong usage: an unknown command or flag, a missing argument. | `repose <command> --help`                                       |
| 3    | Not logged in.                                               | `repose login`                                                  |
| 4    | No such project.                                             | `repose projects` lists yours.                                  |
| 5    | The machine isn't running.                                   | `repose start`                                                  |
| 6    | The machine has uncommitted changes, so the sync stopped.    | See [below](#the-sync-stopped-because-the-machine-has-changes). |
| 7    | No card on file, or a payment problem.                       | The dashboard's Billing page.                                   |
| 8    | No capacity right now.                                       | Try again in a few minutes.                                     |
| 10   | The configuration build failed.                              | Fix the configuration; the error names the line.                |
| 130  | You pressed `Ctrl-C`.                                        |                                                                 |

## Logging in and connecting

### ``Not logged in. Run `repose login`.``

Your login expired or you logged out. Run `repose login`.

### ``No repose project for github.com/you/app. Run `repose run` here to create one, or name one: ...``

This checkout has no project yet, or its remote changed. `repose run` creates one. If the project exists under another remote, name it: `repose attach NAME`.

### `This directory has no git remote. Pass --name NAME to create a project anyway.`

See [Projects](/docs/projects#directories-with-no-remote).

### `ssh todo-app.repose` says `Permission denied`

Your SSH certificate is older than 12 hours. Any of `repose run`, `repose attach`, `repose open` or `repose cp` gets a new one. Run `repose attach todo-app`, detach, and try again.

If the CLI itself works but plain `ssh` doesn't, the `Include` line may be missing from `~/.ssh/config`. The CLI prints a message when it couldn't add it:

```
Could not add the Include line to ~/.ssh/config (...), so `ssh <project>.repose` will not work from a plain terminal. Add this line at its very top:
    Include ~/.ssh/repose/config
```

### ``Could not run ssh ... repose needs the OpenSSH client (`ssh`) on your PATH.``

Install OpenSSH. On Debian and Ubuntu: `sudo apt install openssh-client`.

### `Guest is running but SSH did not answer in 60s.`

The machine booted but its SSH server isn't answering. `repose logs --kind console` shows the boot log. `repose start` restarts a machine that's stuck.

### `The gateway refused a certificate issued just now`

Log in again with the account that owns the project (`repose login`) and retry. If it still fails, your certificates may have been revoked.

## Machine state

### ``todo-app is stopped. Start it with `repose start todo-app`, ...``

`attach`, `open`, `cp` and `status`'s process list need a running machine. `repose start todo-app`, or `repose run` in the checkout.

### `todo-app is in an error state: ...`

The message says why. `repose start todo-app` restarts it in most cases. If that fails too, `repose logs todo-app --kind console` shows what the machine printed while booting.

### `the environment's agent (guestd) stopped answering`

guestd is the small service on each machine that repose talks to. If it stops responding, `repose start` restarts the machine. A manual snapshot or a disk resize can't run until it's back.

### `No capacity right now; try again in a few minutes. (We have been alerted.)`

The servers are full. Nothing was created or changed. Try again later.

## Syncing

### The sync stopped because the machine has changes

```
The guest's working tree has uncommitted changes (3 files):
...
```

Something on the machine changed files since your last sync, usually an agent. Pick one:

- `repose attach` and look.
- `repose run --stash-remote` keeps the machine's changes in `git stash` there.
- `repose run --discard-remote` throws them away.

### `The guest's main has commits your laptop does not have`

An agent committed on the machine. Your laptop's commit was checked out detached on the machine, and the agent's branch was left alone. Push from the machine, pull on the laptop, then run again.

### `Not sent: web/node_modules`

Dependency directories never travel. Run your install command on the machine.

### A big file wasn't sent

Files over 100 MB, and untracked files past 500 MB per sync, are skipped with a warning. Commit what matters, or add large directories to `.gitignore` or [`sync.exclude`](/docs/cli-config).

## Agents

### `Claude Code is not logged in on this guest yet.`

Log in once in the window the CLI opened. See [Agents](/docs/agents#claude-code).

### An agent is stuck waiting

Attach and look. It's usually a permission prompt. To stop agents asking, see [Letting an agent run without asking](/docs/agents#letting-an-agent-run-without-asking).

### A secret isn't set in the agent's environment

Programs read their environment when they start. After `repose secrets set`, start a new tmux window or a new agent.

## Configuration

### `config error: ...` or a build failure (exit 10)

The build failed and nothing changed on the machine. The message shows the Nix error and, where it can, the line in your fragment. Fix it with `repose config edit` or the dashboard's Nix tab. `repose logs --kind build` shows the full log.

### `This change needs a reboot`

Run `repose stop && repose start` when no agent is in the middle of something.

### `command not found` on the machine

The machine tells you which package has it. See [Packages and configuration](/docs/config).

## Ports

### My dev server isn't on localhost

- Are you attached? Automatic forwarding runs only while `repose run` or `repose attach` is connected.
- Is it listening on `127.0.0.1` or `0.0.0.0`? A server bound only to another address isn't forwarded.
- Is the port 1024 or higher?
- Is `REPOSE_NO_FORWARD` set in your shell?
- Look at the tmux status bar: the port may have moved to another number because it was taken on your laptop.

`repose open PORT` forwards one port by hand in the meantime.

## Notifications

### Nothing arrives

Run `repose notify test`. If a channel reports `error`, fix its settings. If both say `ok`, check `repose events` to see whether the event happened at all, and remember the limit of 30 notifications per project per hour.

## Getting more detail

- `-v` on any command prints debug logs to stderr.
- `REPOSE_TIMING=1` prints how long each step of `run` and `attach` took.
- `repose logs --kind ops` lists every operation on the project with its result.
