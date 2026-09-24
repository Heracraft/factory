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

A machine runs until you stop it; repose never stops one for being idle. For one project in detail, including which processes are listening on ports:

```
repose status todo-app
repose status todo-app --watch
```

The dashboard shows the same, plus events, snapshots and a projected monthly cost.

## Stop and start

```
$ repose stop todo-app
Stopped todo-app in 38s. Snapshot 0192… (2.1 GB). Disk is still billed; `repose destroy todo-app` to stop that.

$ repose start todo-app
todo-app is running (large), ready in 9s. `repose attach todo-app` to get in.
```

Stopping ends every process and snapshots the disk (`--no-snapshot` skips that). The disk stays, with everything in `/home/dev`. A stopped machine costs only its disk. `repose run` in the checkout starts a stopped machine too.

`repose start` is also the fix for a project in the `error` state: it restarts the machine on its newest configuration.

## Snapshots

The disk is snapshotted every night while running, and whenever you stop. Take one yourself before something risky:

```
repose snapshots create
repose snapshots list
```

The seven newest are kept, free. A snapshot holds the whole disk (checkout, home directory, logins made on the machine, installed tools) but not [secrets](/docs/secrets), which live only in memory.

To put a project back to a snapshot, stop it first. Stopping takes its own snapshot, so this can be undone:

```
repose stop todo-app
repose snapshots restore SNAPSHOT_ID --project todo-app
```

Or restore into a new project and leave the original alone:

```
repose snapshots restore SNAPSHOT_ID --as-new todo-app-yesterday
```

## Destroy and restore

```
$ repose destroy todo-app
Destroy todo-app? A final snapshot is kept for 30 days. [y/N] y
Destroying todo-app. Bring it back within 30 days with: repose restore todo-app
```

This deletes the machine and its disk and stops all charges for the project. `--yes` skips the question; `--wait` waits until it's done.

Within 30 days, bring it back, running, with its size, configuration and git remote:

```
repose projects --destroyed
repose restore todo-app
```

`--as NEW-NAME` restores under another name, and `--snapshot ID` picks an older snapshot. After 30 days the snapshot is deleted.

## A second machine for the same repository

For an experiment that shouldn't touch your main project, create another one by name:

```
repose run --name todo-app-experiment
```

Commands in the checkout still mean the original; reach the new one by name. This is also how to run several agents on one repository without them sharing a working tree.

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
