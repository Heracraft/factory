# Stop, start, destroy

A project's guest is always on until the user stops it (DECISIONS R1-5).
Stopping snapshots and deallocates; the disk stays and keeps being billed.
Destroying deletes the disk and keeps the last snapshot for 30 days.

## What the user sees

Every command takes the project as its argument, or finds the checkout's
(DECISIONS I-155). While it waits, stderr shows the phase with a spinner
and elapsed time on a terminal, or one line per phase elsewhere (I-154).

```
$ repose stop todo-app
Snapshotting and stopping todo-app...
Stopped todo-app in 38s. Snapshot 0192… (2.1 GB). Disk is still billed; `repose destroy todo-app` to stop that.

$ repose stop --no-snapshot
Stopped todo-app in 6.2s. Disk is still billed; `repose destroy todo-app` to stop that.

$ repose start todo-app
Starting todo-app...
todo-app is running (large), ready in 4.1s. `repose attach todo-app` to get in.

$ repose start age-calculator          # in `error`: the api restarts it (I-157)
Restarting age-calculator (its agent stopped answering)...
age-calculator is running (large), ready in 21s. `repose attach age-calculator` to get in.

$ repose destroy todo-app
Destroy todo-app? A final snapshot is kept for 30 days. [y/N] y
Destroying todo-app. Bring it back within 30 days with: repose restore todo-app

$ repose projects --destroyed
PROJECT   CLASS  DESTROYED         SNAPSHOT          SIZE    RESTORABLE UNTIL
todo-app  large  2026-09-23 02:23  2026-09-23 02:23  2.0 MB  2026-10-23
`repose restore NAME` brings one back (`--as NEW-NAME` when the name is in use).

$ repose restore todo-app
Restored todo-app from its snapshot of 2026-09-23 02:23 in 31s; it is running (large). `repose attach todo-app` to get in.
```

The destroy returns as soon as the api has accepted it (DECISIONS
I-166); the project reads `destroying` until it is gone. A destroy that
fails shows in `repose projects` and `repose status` as `error` with the
reason and the retry, and as a `destroy_failed` notification (I-165).
`--wait` waits and reports, for scripts, and never prints "Destroyed"
for a destroy that failed (I-153):

```
$ repose destroy age-calculator --yes --wait
Could not destroy age-calculator: the host could not remove the volume (internal). age-calculator is still there, in state error. `repose destroy age-calculator` tries again.
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
| stopped | none | kept | GB-month only | no (`repose start`) |
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
  with `todo-app is stopped; run \`repose start\`` from the moment the
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
  says to `start` or `run`. Every command that needs a running guest names
  the real state (stopped, still building, stopping, in `error` with the
  reason the api recorded) and the command that fits it (I-153).
- `start` is also the recovery path (I-157). On a project in `error`, or a
  running one whose guestd (the agent inside the environment) has stopped
  answering, it restarts: the unit is stopped without a snapshot (nothing
  can freeze the filesystem without guestd), the newest built revision is
  put in place while the guest is down, and the guest boots on it. The
  response says `restart: true`; the CLI says it is restarting. A running
  project with a healthy guestd still answers "already running".

Destroy:

- Asks `Destroy <slug>? A final snapshot is kept for 30 days. [y/N]`
  unless `--yes` (`-y`); an empty answer is no, and without a terminal the
  CLI asks for `--yes` rather than guessing (the owner's request,
  2026-09-23: the snapshot makes a typed name redundant). The CLI returns
  once the api has accepted the destroy and prints `repose restore
  <slug>` (I-166); `--wait` waits for the op and reports `Destroyed` only
  when it is done and the project is gone, and a failed op with the
  project's state and the command that retries (DECISIONS I-153,
  api.md).
- The project is `destroying` from the moment the api accepts. The op
  stops the guest if it is not stopped (no snapshot in the stop), then
  takes the final snapshot of the stopped volume, which is clean without
  a freeze, then deletes the guest (I-165). The snapshot reads only the
  blocks the filesystem uses (I-164), so its time follows the data, not
  the volume's size.
- Deletes the thin volume, the guest's units, GC roots for its closures,
  its tap and nftables entries, and the tmpfs secrets. Keeps the project row
  (`destroyed_at` set), its events, its usage, and its newest snapshot with
  `expires_at` 30 days out.
- Frees the project slot immediately for the account's limit.
- Always finishes once asked (I-156). If guestd is dead the guest cannot
  be frozen, so the unit is stopped (the hypervisor's shutdown, then a
  kill) and the final snapshot is taken of the stopped volume, which is
  crash-consistent at worst. If even that is impossible the snapshot is
  skipped with a `snapshot_failed` event and the newest earlier snapshot
  is the one kept 30 days. A guest the host no longer has is already
  destroyed. The op ends `error` only for a real host failure (deleting
  the volume, uploading the snapshot), and destroying again resumes.
- `DELETE` answers with the destroy's `op_id`; `repose destroy --wait`
  waits for that op and prints "Destroyed." only when it is `done`. A
  failed destroy leaves the project in `error` with a reason that names
  `repose destroy <slug>` as the retry, and sends `destroy_failed`.
- Within 30 days, `repose restore <slug>` brings it back as a new project
  under the same name (or `--as NEW-NAME` when a live project has it),
  from its newest snapshot or `--snapshot ID`, with its class, volume
  size, configuration and remote (I-167). `repose projects --destroyed`
  and the dashboard's "Recently destroyed" list what can be restored and
  until when. `repose snapshots restore <id> --as-new <name>` still
  works. After 30 days the snapshot is deleted by the retention job and
  neither lists it.

Account cancellation (`DELETE /me`, dashboard button):

- Every running guest is stopped with a snapshot; every project is marked
  destroyed; snapshots kept 30 days; billing closes at the end of the
  period; after 30 days the user's data is deleted except invoices and the
  audit log, which the terms say are retained.

Failure handling:

- A guest that fails to boot goes to `error` with the console log's last
  50 lines attached to the op; `repose logs --kind console` shows them.
  The volume is untouched; `start` retries; `snapshots restore` is the
  escape.
- A dead guestd never makes an op fail instantly for ever. `stop` stops
  the unit anyway and snapshots the stopped volume; `destroy` finishes as
  above; `start` restarts. A manual snapshot and a resize need guestd (to
  freeze, to grow the filesystem) and do not reboot the guest unasked:
  they fail with a message that says to run `repose start`, and succeed
  when run again after it (I-158).
- An op's error is a sentence with the way out, and its code: code
  `guest_unresponsive`, message "the environment's agent (guestd) stopped
  answering; `repose start` restarts it". The host's own wording, with
  internal ids, is the error's `detail` (I-159).
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
