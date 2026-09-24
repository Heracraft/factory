---
title: Stop, start, destroy
description: What each state costs, snapshots, and bringing a project back.
section: Using repose
order: 20
---

A machine runs until you stop it. repose never stops a machine for being idle. That's deliberate, since the point is to leave an agent working, but it means a machine you forget about keeps costing money. `repose projects` shows what's running.

## Stop

```
$ repose stop todo-app
Stopped todo-app in 38s. Snapshot 0192… (2.1 GB). Disk is still billed; `repose destroy todo-app` to stop that.
```

Stopping shuts the machine down, which ends every process on it: agents, dev servers, the tmux session. Then it takes a snapshot of the disk. The disk itself stays, with everything in `/home/dev`.

`--no-snapshot` skips the snapshot and is faster. The newest snapshot is then the last nightly one.

A stopped machine is charged for its disk only. See [Pricing](/docs/billing).

## Start

```
$ repose start todo-app
todo-app is running (large), ready in 14s. `repose attach todo-app` to get in.
```

`repose run` in the checkout starts a stopped machine too, then syncs and attaches as usual. `repose attach` and `repose open` don't start anything; they say the machine is stopped and which command to run.

`start` is also the fix for a machine in the `error` state, or one whose status says "the environment's agent (guestd) is not answering". It restarts the machine without a snapshot and boots it on the newest built configuration:

```
$ repose start age-calculator
Restarting age-calculator (its agent stopped answering)...
age-calculator is running (large), ready in 21s. `repose attach age-calculator` to get in.
```

## States

| State                  | What it means                                                                 | Billed for            |
| ---------------------- | ----------------------------------------------------------------------------- | --------------------- |
| `creating`, `building` | A new project's disk and environment are being made.                          | Nothing               |
| `starting`             | Booting.                                                                      | Disk                  |
| `running`              | On and reachable.                                                             | Compute, disk, egress |
| `stopping`             | Shutting down and snapshotting.                                               | Compute until stopped |
| `stopped`              | Off. The disk is kept.                                                        | Disk                  |
| `restoring`            | A snapshot is being written onto the disk.                                    | Disk                  |
| `destroying`           | Being deleted, final snapshot first.                                          | Until it's gone       |
| `error`                | Something failed. `repose status` says what; `repose start` usually fixes it. | Disk                  |

Compute is counted by the minute while the state is `running`.

## Snapshots

A running project's disk is snapshotted every night at 03:00 server time, and every project's disk is snapshotted when you stop it. You can take one yourself at any time:

```
$ repose snapshots create
Snapshot of todo-app taken in 41s.
```

Taking a snapshot of a running machine freezes its filesystem for under a second. Agents keep running. A database in the middle of a write gets the same copy it would get from a power cut, which databases are built to recover from.

```
$ repose snapshots list
ID          TAKEN             SIZE     REASON
snap_01J8…  2026-09-17 03:00  2.1 GB   scheduled
snap_01J8…  2026-09-16 22:14  2.0 GB   stop
```

The seven newest snapshots are kept; a manual one counts toward the seven. The oldest is deleted only after a newer one has succeeded. A project that's been stopped for a long time keeps its latest snapshot however old it is. Snapshots are free while the project exists.

Snapshots contain the whole disk: your checkout, your home directory, logins you made on the machine, `.env` files, installed tools. They don't contain [named secrets](/docs/secrets), which only ever live in memory.

### Restoring a snapshot

Into the same project, which replaces its disk. The machine has to be stopped first:

```
$ repose stop todo-app
$ repose snapshots restore snap_01J8… --project todo-app
Restore over the current volume? Anything since the snapshot is lost. [y/N] y
Restored todo-app. `repose start todo-app` boots it.
```

Stopping takes a snapshot first, so even this is reversible.

Or into a new project next to the original, which leaves the original alone:

```
$ repose snapshots restore snap_01J8… --as-new todo-app-yesterday
Restored into a new project, todo-app-yesterday. `repose projects` lists it.
```

The dashboard's project page lists snapshots with **Restore** and **Restore as new…** buttons, and a **Create** button.

## Destroy

```
$ repose destroy todo-app
Destroy todo-app? A final snapshot is kept for 30 days. [y/N] y
Destroying todo-app. Bring it back within 30 days with: repose restore todo-app
```

Destroying stops the machine if it's running, takes a final snapshot, and deletes the machine and its disk. Billing for the project stops. The final snapshot is kept for 30 days at no charge, and the project's slot is free for a new one at once.

The command returns as soon as the destroy has started. Pass `--yes` to skip the question (a script with no terminal must), and `--wait` to wait until it's finished. If a destroy fails, the project shows as `error` and you get a `destroy failed` notification; running `repose destroy` again picks it up.

In the dashboard, the **Destroy** section at the bottom of the project page asks you to type the project's name.

## Restore a destroyed project

```
$ repose projects --destroyed
PROJECT   CLASS  DESTROYED         SNAPSHOT          SIZE    RESTORABLE UNTIL  EARLIER
todo-app  large  2026-09-23 02:23  2026-09-23 02:23  2.0 GB  2026-10-23

$ repose restore todo-app
Restored todo-app from its snapshot of 2026-09-23 02:23 in 31s; it is running (large). `repose attach todo-app` to get in.
```

The project comes back with its size, disk size, configuration and git remote, running. Inside the checkout, plain `repose restore` finds it by the remote. If a live project already has the name, restore under another with `--as NEW-NAME`. `--snapshot ID` restores an older snapshot instead of the newest.

After 30 days the snapshot is deleted and the project can't be restored.

## Deleting your account

The Account page in the dashboard has **Delete account**. It stops every machine at once and marks every project destroyed. Snapshots are kept for 30 days, then deleted along with the rest of your data, except invoices and the audit log, which the [terms](/terms) say are retained.
