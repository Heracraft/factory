---
title: Status, logs and the dashboard
description: Checking what a project is doing and what it costs, from the terminal or the browser.
section: Using repose
order: 21
---

## `repose status`

Shows one project in detail: this checkout's, or the one you name.

```
$ repose status todo-app
todo-app   large  running   2h14m   claude: working      today $0.31   month $18.40
  host az-eastus-01   ip 10.64.0.12   disk 6.2 GB/40.0 GB   snapshot 9h14m ago
  sessions 1   tmux clients 1   docker 2
  last event 14m ago: claude completed "Added the rate limiter and its tests."
  listening  node :5173 up 3d 410.0 MB
             postgres :5432 up 3d 38.1 MB
```

Agent states are `working`, `idle`, `needs_input` and `unknown`. They come from the machine and are at most a minute old.

The `listening` lines are read from the machine over your own SSH connection when you run the command, and only then. repose doesn't record them. If the machine doesn't answer within a few seconds, those lines are left out and the rest still shows.

Costs are today and the month to date, including an estimate for the hour in progress. Usage is totalled hourly, a few minutes past each hour, so the figure can trail by an hour or so.

`--watch` refreshes every 5 seconds. `--json` prints the full project record.

`status` exits 0 even when a project is in `error`. The state is the answer.

## `repose projects`

Every project in one table, from anywhere:

```
$ repose projects
PROJECT    CLASS  STATE    UP     AGENTS           TODAY  MONTH
todo-app   large  running  2h14m  claude: working  $0.31  $18.40
api-v2     xl     stopped  -      -                $0.00  $41.02
scratch    small  running  6d3h   -                $1.63  $9.88
```

A project in `error` gets a line underneath with the reason and the command that fixes it:

```
age-calculator: the environment's agent (guestd) stopped answering; `repose start age-calculator` restarts it.
```

`--json` prints the full records. `--destroyed` lists destroyed projects that can still be restored. See [Stop, start, destroy](/docs/lifecycle#restore-a-destroyed-project).

## `repose logs`

```
repose logs                  # the machine's console
repose logs --kind build     # the last configuration build
repose logs --kind ops       # create, start, stop, apply and snapshot history
repose logs todo-app -f      # keep following
repose logs --since 1h
```

The console log is the machine's boot and kernel output. It's where to look when a machine won't start. It doesn't include your applications' output; that stays on the machine wherever your app writes it. repose never reads anything under `/home/dev`.

The build log is the complete output of the last configuration build, with secret values replaced by `[redacted]`.

Console logs are kept 30 days, the logs of the 20 most recent builds are kept, and the operations history lasts as long as the project does. The [privacy policy](/privacy) lists every retention period.

## `repose events`

```
repose events todo-app
```

Notification-worthy events from the last 24 hours: agents finishing, asking for input, errors, snapshots failing. `--since 72h` goes further back, `-f` polls every 10 seconds, `--json` prints raw records. See [Notifications](/docs/notifications).

## The dashboard

[repose.herakraft.co](https://repose.herakraft.co), signed in with the same GitHub account.

**Projects** lists every project with its size, state, agent state and cost today and this month. Below it, **Recently destroyed** lists projects you can still restore.

A **project's page** shows:

- Start or Stop (with a **Snapshot on stop** box you can untick);
- the `repose run` and `ssh <project>.repose` lines to connect;
- live signals: SSH sessions, tmux clients and each agent's state;
- cost today, this month, and the month projected at the current rate;
- disk used and allocated, with **Resize…** to grow it;
- the last build's status, with a link to the configuration page;
- events, newest first;
- snapshots, with Create, Restore and Restore as new;
- Destroy, which asks you to type the project's name.

Links at the top go to the project's **Config** page (menu, Nix editor, build log, revisions, holding base updates; see [Packages and configuration](/docs/config)) and its **Secrets** page (add, list and delete [secrets](/docs/secrets)).

**Billing** has your card, invoices and this month's usage. **Settings** has your time zone, email and ntfy notifications and the install command. **Account** has your handle, email and GitHub login, and account deletion.
