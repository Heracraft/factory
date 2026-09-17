# Stop, start, destroy

A project's guest is always on until the user stops it (DECISIONS R1-5).
Stopping snapshots and deallocates; the disk stays and keeps being billed.
Destroying deletes the disk and keeps the last snapshot for 30 days.

## What the user sees

```
$ factory stop
Snapshotting todo-app ... 2.1 GB in 38s
Stopping ... stopped. Disk (40 GB) is kept and billed at $0.10/GB-month.

$ factory stop --no-snapshot
Stopping ... stopped.

$ factory start
Starting todo-app ... 4s. Attach with `factory attach`.

$ factory destroy
This deletes todo-app's disk. The last snapshot (2026-09-17 03:00, 2.1 GB)
is kept for 30 days and can be restored with `factory snapshots restore
--as-new`. Type the project name to confirm: todo-app
Destroyed.
```

## States

`creating → building → starting → running → stopping → stopped → starting
…`, plus `restoring`, `destroying`, `destroyed`, `error`. The enum is in
`interfaces/README.md` and the CLI shows the same words.

| State | Guest | Volume | Billed | Reachable |
|---|---|---|---|---|
| creating, building | none yet | allocating | no | no |
| starting | booting | attached | guest-hours from `running` | no |
| running | on | attached | guest-hours + GB-month + egress | yes |
| stopping | shutting down, snapshotting | attached | until `stopped` | no |
| stopped | none | kept | GB-month only | no (`factory start`) |
| restoring | none | being rewritten | GB-month | no |
| destroying | none | deleting | until `destroyed` | no |
| destroyed | none | gone | nothing; last snapshot kept 30 days at no charge | no |
| error | may be on | attached | as `stopped` | maybe |

Guest-hours accrue per minute of `running` and are rounded up to the minute,
not the hour. PRICING.md has the rates.

## Behaviour that must hold

Stop:

- `stop` sends `StopGuest{snapshot_first: true, timeout_s: 60}`. guestd gets
  `Shutdown`; systemd in the guest stops services, which includes tmux and
  any agent in it. The agent is interrupted; a Claude session can be resumed
  in the guest after `start` with `claude --resume`, and the CLI says so
  when it detects an agent window was open. After 60 seconds without a
  clean shutdown, hostd shuts the VM down through Cloud Hypervisor.
- The snapshot happens after the guest is down, so it is clean, not merely
  crash-consistent. `--no-snapshot` skips it and prints that the newest
  snapshot is now the last nightly one.
- SSH sessions to a stopping guest are closed; the gateway rejects new ones
  with `todo-app is stopped; run \`factory start\`` from the moment the
  state leaves `running`.
- A stopped project's secrets, config, and events are all retained and
  visible.

Start:

- `start` on a stopped project boots the same volume on the same host. If
  that host is `unreachable` or `retired`, the CLI says so and offers
  `start --restore-latest`, which restores the newest snapshot onto another
  host as the same project. This is the host-loss path and it is rehearsed.
- Pending config revisions marked `--later` apply during start, and base
  bumps that needed a reboot apply here too; the CLI prints what changed.
- Start is refused with `payment_required` when billing is `past_due` for
  more than 3 days or `suspended`, with the dashboard billing link.
- `run` on a stopped project starts it implicitly; `attach` does not, and
  says to `start` or `run`.

Destroy:

- Requires typing the project name unless `--yes`. Stops first if running,
  with a final snapshot unless `--no-snapshot`.
- Deletes the thin volume, the guest's units, GC roots for its closures,
  its tap and nftables entries, and the tmpfs secrets. Keeps the project row
  (`destroyed_at` set), its events, its usage, and its newest snapshot with
  `expires_at` 30 days out.
- Frees the project slot immediately for the account's limit.
- Within 30 days, `factory snapshots restore <id> --as-new <name>` brings
  it back as a new project. After 30 days the snapshot is deleted by the
  retention job and the dashboard stops listing it.

Account cancellation (`DELETE /me`, dashboard button):

- Every running guest is stopped with a snapshot; every project is marked
  destroyed; snapshots kept 30 days; billing closes at the end of the
  period; after 30 days the user's data is deleted except invoices and the
  audit log, which the terms say are retained.

Failure handling:

- A guest that fails to boot goes to `error` with the console log's last
  50 lines attached to the op; `factory logs --kind console` shows them.
  The volume is untouched; `start` retries; `snapshots restore` is the
  escape.
- hostd restart mid-operation: the op is replayed by the API with the same
  command id and completes or is idempotently skipped
  (`interfaces/grpc-hostd.md`). No state is lost.

## Depends on

Workstreams 03 (StopGuest, StartGuest, DestroyGuest, cleanup, replay), 04
(Shutdown), 05 (state machine, ops, retention job, cancellation), 07
(commands and prompts), 09 (billing states gating start), 06 (rejecting
sessions to non-running guests).

## Deferred

Idle auto-stop (R1-5; needs the recorded signals). Scheduled start and stop.
Pause (Cloud Hypervisor pause without snapshot) as a cheaper stop.
