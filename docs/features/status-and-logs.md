# Status and logs

`repose status` answers "what is it doing and what is it costing" in one
screen. `repose logs` shows the guest's console, the last build, or the
operations history. The dashboard shows the same data with history.

## What the user sees

```
$ repose status
PROJECT     CLASS  STATE    UP       AGENTS                   TODAY    MONTH
todo-app    large  running  2h14m    claude: working          $0.31    $18.40
api-v2      xl     stopped  -        -                        $0.00    $41.02
scratch     small  running  6d3h     -                        $1.63    $9.88

$ repose status --project todo-app
todo-app  large  running on az-eastus-01  up 2h14m
  base      2026.09.15 (held; latest 2026.09.22)
  config    r14 applied 2026-09-17 13:40
  git       main @ 3f9e2a1, tree dirty (2 files)
  agents    claude in todo-app:claude: working (last event 14m ago: completed "Added auth flow…")
  ports     3000, 5432
  disk      40 GB allocated, 6.2 GB used
  snapshot  2026-09-17 03:00 (2.1 GB)
  sessions  1 ssh, 1 tmux client
  cost      today $0.31 · month $18.40 · cap $99.00
```

```
$ repose logs                    # console, last 200 lines, follow with -f
$ repose logs --kind build       # the last build's output
$ repose logs --kind ops         # create/start/stop/apply/snapshot history
```

## Behaviour that must hold

Status:

- The table view lists every non-destroyed project with class, state,
  uptime since the last `running` transition, per-agent state, and cost
  today and month to date in dollars from `usage_hours` plus the current
  partial hour estimated at the class rate.
- Agent state per window comes from guestd's latest `AgentState`
  (`working`, `idle`, `needs_input`, `unknown`) and is at most 60 seconds
  stale on a healthy guest; older than 5 minutes is shown as `?` with the
  age.
- Git state is read by guestd (`branch`, `HEAD`, dirty count) as part of
  `Sample`, so the user knows work is waiting to be committed without
  attaching.
- Listening ports come from guestd's `ss -ltn`; the CLI shows them so the
  user knows what to `open`.
- `status` never triggers a certificate refresh or an SSH connection; it is
  API only and works when the guest is unreachable, showing the last known
  data with its age.
- `--json` prints the `Project` object from `interfaces/api.md` verbatim.
- Exit code is 0 even when a project is in `error`; the state is the
  information. `status --project X` on an unknown project exits 4.

Logs:

- `console`: the guest's serial console as captured by hostd, last 200
  lines by default, `-f` follows over SSE. It contains boot messages and
  kernel output, not application logs; the doc says where application logs
  are (in the guest, wherever the app writes them).
- `build`: the most recent config build's output, complete, with secret
  values redacted (secrets.md).
- `ops`: one line per operation with timestamps, duration, and result;
  errors expanded.
- Log lines are the platform's own; nothing is read from inside
  `/home/dev`. A user's application logs are theirs and stay in the guest.
- Retention: console 7 days, build logs 90 days, ops forever with the
  project row.

Dashboard (as built; `workstreams/08-dashboard.md` §5.2 is the page list
this describes, and it is narrower than an earlier draft of this section
promised, DECISIONS I-96):

- `/projects`: one row per project — name, class, state, uptime, agent
  state, cost today, cost this month. The same figures as `status`, from
  the same `usage_hours` rows. Not sortable, and no sparkline.
- `/projects/[id]`: cards for connect (the `repose run` and `ssh` lines),
  signals, cost (today, this month, and the month projected at the
  current run rate), disk with a resize control, events newest first, the
  last build with a link to the config page, and snapshots with restore
  and restore-as-new. Start, Stop, Resize and Destroy are the header
  actions; Destroy makes you type the slug.
- `/projects/[id]/config` and `/projects/[id]/secrets` are their own
  pages, not cards: the config page carries the menu, the Nix editor, the
  streaming build log and the revision list, and the secrets page the
  names and their dates.
- `/billing`: card on file, invoices, usage for the month by class.
  `/settings`: timezone, email toggle, ntfy URL and its test button.
  `/account`: handle, email, GitHub login, and deletion.

## Depends on

Workstreams 05 (project and usage routes, ops history, log storage), 03
(console capture, build logs), 04 (signals including git and ports), 07
(`status`, `logs`), 08 (dashboard pages), 09 (cost figures).

## Deferred

Application log shipping from the guest (opt-in). Historical resource
graphs per project in the dashboard beyond cost. A `repose top` live view.
A state timeline and a per-meter cost breakdown on the project page, a
sortable project list with a cost sparkline, a ports card, and the account
limits on a page of their own: each was in an early draft of the Dashboard
section above and none is built (DECISIONS I-96).
