# Status and logs

`factory status` answers "what is it doing and what is it costing" in one
screen. `factory logs` shows the guest's console, the last build, or the
operations history. The dashboard shows the same data with history.

## What the user sees

```
$ factory status
PROJECT     CLASS  STATE    UP       AGENTS                   TODAY    MONTH
todo-app    large  running  2h14m    claude: working          $0.31    $18.40
api-v2      xl     stopped  -        -                        $0.00    $41.02
scratch     small  running  6d3h     -                        $1.63    $9.88

$ factory status --project todo-app
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
$ factory logs                    # console, last 200 lines, follow with -f
$ factory logs --kind build       # the last build's output
$ factory logs --kind ops         # create/start/stop/apply/snapshot history
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

Dashboard:

- Project list with the same columns, sortable, with cost sparkline for
  the month.
- Project page: state timeline, agent events, build history with logs,
  snapshots, secrets (names), config (menu or fragment), ports with (later)
  preview links, and a cost breakdown by meter.
- Account page: card status, invoices, notification channels, limits.

## Depends on

Workstreams 05 (project and usage routes, ops history, log storage), 03
(console capture, build logs), 04 (signals including git and ports), 07
(`status`, `logs`), 08 (dashboard pages), 09 (cost figures).

## Deferred

Application log shipping from the guest (opt-in). Historical resource
graphs per project in the dashboard beyond cost. A `factory top` live view.
