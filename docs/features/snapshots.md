# Snapshots

Every project's volume is snapshotted nightly and on every stop, streamed to
Azure Blob, and kept for seven days. A snapshot restores into a fresh
volume on any host, which is also how a project moves hosts and how it
survives a host dying.

## What the user sees

```
$ repose snapshots list
ID          TAKEN                 SIZE     REASON
snap_01J8…  2026-09-17 03:00 UTC  2.1 GB   scheduled
snap_01J8…  2026-09-16 22:14 UTC  2.0 GB   stop
snap_01J8…  2026-09-16 03:00 UTC  1.9 GB   scheduled

$ repose snapshots create
Snapshotting todo-app ... 2.1 GB uploaded in 41s (snap_01J8…)

$ repose snapshots restore snap_01J8…
todo-app is running. Restoring replaces its current disk. Stop it first?
[y/N] y
Stopping (with a final snapshot) ... restoring 2.1 GB ... starting ... done.

$ repose snapshots restore snap_01J8… --as-new todo-app-yesterday
Created todo-app-yesterday (large) from snap_01J8… on az-eastus-01 ... done.
```

## Behaviour that must hold

Taking (DECISIONS R3-6):

- Schedule: 03:00 in the host's timezone for every running project, and on
  every `stop` unless `--no-snapshot`. Manual with `snapshots create`.
- Consistency: guestd freezes the filesystem, hostd takes an LVM thin
  snapshot, guestd thaws. The frozen window is under one second; a test
  measures it. If the thaw does not arrive within 10 seconds guestd thaws
  itself and reports a warning, because a guest frozen for a minute looks
  like a hung agent.
- The LVM snapshot is streamed zstd-compressed to Blob at
  `<user>/<project>/<timestamp>.img.zst`, then removed. Only used blocks
  travel (thin snapshots know which blocks are allocated), so a 40 GB volume
  with 2 GB used uploads about 2 GB.
- A snapshot writes a `snapshots` row with size and reason only after the
  upload has been verified by size and checksum against Blob.
- A running agent is not paused for a snapshot. Docker containers are not
  paused. A database mid-write in the guest gets a crash-consistent copy,
  which is what a power cut would give; the doc says so.

Retention (DECISIONS R4-11):

- Seven daily snapshots per project, oldest deleted after the newest
  succeeds, never before. Manual snapshots count toward the seven.
- After `destroy`, the last snapshot is kept 30 days and listed under the
  destroyed project in the dashboard; `restore --as-new` brings it back.
- After account cancellation, all guests stop, snapshots are kept 30 days,
  then deleted with the account's other data.
- A project that has been stopped for months keeps its most recent
  snapshot indefinitely (the seven-day window only rolls while new
  snapshots are taken), because the volume it backs is still billed and
  still exists.

Restoring:

- `restore` into the same project requires the guest to be stopped; the CLI
  offers to stop it, which takes a final snapshot first so the restore is
  reversible.
- `restore --as-new NAME` creates a new project with the same class and
  volume size, on the host with the most free memory, and starts it. It
  counts toward the project limit.
- Restore works onto any host, not only the one the snapshot came from. The
  release checklist rehearses exactly that.
- A restore verifies the download checksum before writing the volume, and
  a failed restore leaves the target volume untouched.

Alerts:

- A running project whose newest snapshot is older than 36 hours raises an
  operator alert, because it means the nightly job failed twice.

## Depends on

Workstreams 03 (snapshot and restore commands, LVM, Blob upload), 04
(Freeze, Thaw), 05 (snapshots routes, retention job, scheduling), 07
(`snapshots` commands), 11 (Blob container, lifecycle rules as a backstop),
10 (snapshot age metric).

## Deferred

Incremental uploads (block-level deltas between snapshots). User-set
schedules and retention. Cross-region copies. Restoring a single file or
directory from a snapshot.
