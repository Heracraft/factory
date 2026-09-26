---
title: Projects and lifecycle
description: Check on projects, stop and start them, take and restore snapshots, destroy and bring back.
section: Using repose
order: 17
---

A project is one machine plus its disk, snapshots, secrets and configuration. The first `repose run` in a checkout creates it. After that, any checkout of the same repository, on any laptop you're logged in on, finds it by its git remote.

To act on a project from elsewhere, name it: `repose attach todo-app`, `repose stop todo-app`. For `repose run`, whose argument is the prompt, use `--project todo-app`.

## See what's running

```
$ repose projects
PROJECT    CLASS  STATE    UP     AGENTS           TODAY  MONTH
todo-app   large  running  2h14m  claude: working  $0.31  $18.40
api-v2     xl     stopped  -      -                $0.00  $41.02
```

A machine runs until you stop it; repose never stops one for being idle. It does tell you when one is, see [Idle machines](#idle-machines). For one project in detail, including which processes are listening on ports:

```
repose status todo-app
repose status todo-app --watch
```

The dashboard's project page shows the state, agents, SSH sessions and cost, plus events, snapshots, the last build and a projected monthly cost. It doesn't list listening ports.

## Stop and start

```
$ repose stop todo-app
Stopped todo-app in 38s. Snapshot 0192… (2.1 GB). Disk is still billed; `repose destroy todo-app` to stop that.

$ repose start todo-app
todo-app is running (large), ready in 9s. `repose attach todo-app` to get in.
```

Stopping ends every process and snapshots the disk (`--no-snapshot`, or unticking **Snapshot on stop** in the dashboard, skips that). The disk stays, with everything in `/home/dev`. A stopped machine costs only its disk. `repose run` in the checkout starts a stopped machine too.

`repose start` is also the fix for a project in the `error` state: it restarts the machine on its newest configuration. The dashboard's **Start** button is there only while a project is stopped.

## Idle machines

A machine is idle when it has been running for 24 hours with no SSH session, no tmux client and no agent working. An agent sitting at its prompt, finished or waiting for you, doesn't count as working. repose doesn't stop an idle machine, because an agent's long job can look the same from outside. It tells you instead:

```
$ repose projects
PROJECT    CLASS  STATE    UP      AGENTS         TODAY  MONTH
todo-app   large  running  31h02m  claude: idle   $3.36  $22.10
todo-app: idle 26h, billing ~$0.14/h; `repose stop todo-app` stops it
```

- `repose status` shows the same line, and the dashboard's project list shows the idle time and rate under the state.
- `repose run` and `repose attach` in another project print one line naming it, once per idle stretch.
- You get one notification, by email and ntfy if you have them on ([Notifications](/docs/notifications)), with the title `todo-app: idle, still billing`. You get another only after the machine has been used and gone idle again.

The rate is the class's hourly price; the month's compute still stops at the class's cap ([Billing](/docs/billing)). When the server hasn't reported on the machine for 10 minutes, for example while it's unreachable, repose can't tell and says nothing.

## Snapshots

The disk is snapshotted every night while running, and whenever you stop. Take one yourself before something risky:

```
repose snapshots create
repose snapshots list
```

Snapshots from the last 7 days are kept, free, and the newest one is always kept. A snapshot holds the whole disk (checkout, home directory, logins made on the machine, installed tools) but not [secrets](/docs/secrets), which live only in memory.

To put a project back to a snapshot, stop it first. Stopping takes its own snapshot, so this can be undone:

```
repose stop todo-app
repose snapshots restore SNAPSHOT_ID --project todo-app
```

Or restore into a new project and leave the original alone:

```
repose snapshots restore SNAPSHOT_ID --as-new todo-app-yesterday
```

The dashboard's snapshot list has **Create**, **Restore** and **Restore as new…** too.

## Destroy and restore

```
$ repose destroy todo-app
Destroy todo-app? A final snapshot is kept for 30 days. [y/N] y
Destroying todo-app. Bring it back within 30 days with: repose restore todo-app
```

This deletes the machine and its disk and stops all charges for the project. `--yes` skips the question; `--wait` waits until it's done. In the dashboard, **Destroy** asks you to type the project's name.

Within 30 days, bring it back, running, with its size, configuration and git remote:

```
repose projects --destroyed
repose restore todo-app
```

`--as NEW-NAME` restores under another name, and `--snapshot ID` picks an older snapshot. A restore started while the destroy is still running waits for it. After 30 days the snapshot is deleted.

The dashboard's project list has the same under **Recently destroyed**, with the date each can be restored until.

## A second machine for the same repository

For an experiment that shouldn't touch your main project, create another one by name:

```
repose run --name todo-app-experiment
```

Commands in the checkout still mean the original; reach the new one by name. This is also how to run several agents on one repository without them sharing a working tree.

## Fork a project

To have several agents try different approaches from the same starting point, each with a machine of its own, fork the project:

```
$ repose fork todo-app -n 3
Forked todo-app into 3 projects from its snapshot of 2026-09-25 14:02 in 48s:
  todo-app-fork-1  running (large)
  todo-app-fork-2  running (large)
  todo-app-fork-3  running (large)
```

`repose fork` snapshots the project and restores the snapshot into new projects. Each copy starts with the same disk: the code and its uncommitted changes, installed dependencies, Docker images, logins made on the machine. It also gets the project's configuration and [secrets](/docs/secrets). Processes don't carry over; each copy boots fresh. The code is at `~/todo-app-fork-1` in the first copy, which links to `~/todo-app`, so paths inside the project keep working.

`--prompt "..."` starts the agent in every copy with the same prompt. To give each copy its own prompt, attach to it and type it, or run `repose run --project todo-app-fork-2 --no-sync "..."`.

The original keeps running and is still the project `repose run` uses in your checkout. Reach the copies by name: `repose attach todo-app-fork-2`. To keep one copy's work, commit it there and push a branch (`git push origin HEAD:try-2`), then fetch it on your laptop. Destroy the copies you don't need with `repose destroy todo-app-fork-1`.

Each copy is a project: it counts toward your [project limit](/docs/limits) and is billed like any project while it runs. If the copies would take you past the limit, `repose fork` creates none of them. `--size small` makes cheaper copies; `--name` changes their names.

## Logs and events

```
repose logs                  # the machine's boot and kernel output
repose logs --kind build     # the last configuration build
repose logs --kind ops       # create, start, stop and snapshot history
repose events                # agent and project events, last 24 hours
```

Your applications' output isn't collected; it stays on the machine.

## States

| State                  | Meaning                                                                       |
| ---------------------- | ----------------------------------------------------------------------------- |
| `creating`, `building` | A new project's disk and environment are being made.                          |
| `starting`             | Booting.                                                                      |
| `running`              | On. Compute is counted by the minute.                                         |
| `stopping`, `stopped`  | Shutting down, or off with the disk kept.                                     |
| `restoring`            | A snapshot is being written to the disk.                                      |
| `destroying`           | Being deleted, final snapshot first.                                          |
| `error`                | Something failed. `repose status` says what; `repose start` usually fixes it. |
