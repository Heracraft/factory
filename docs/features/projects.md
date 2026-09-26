# Projects

A project is one guest, its volume, its snapshots, its config, and its meter
rows, owned by one user. The CLI decides which project a command means from
the directory it runs in.

## What the user sees

```
$ cd ~/code/todo-app
$ repose run
✓ Created todo-app (large)  4s
...
```

```
$ cd ~/scratch/no-remote
$ repose run
This directory has no git remote. Pass --name NAME to create a project anyway.
```

```
$ repose run --name todo-app-experiment
✓ Created todo-app-experiment (large)  4s
...
```

(The class is `--size`, or `default_class` from `config.toml`, `large`
unless set. On a terminal the phase line is a spinner that becomes the ✓
line; elsewhere it prints `Creating todo-app...`.)

## Behaviour that must hold

Identity:

- The CLI reads `git remote get-url origin` and normalises it as
  `interfaces/cli-config.md` says. `git@github.com:A/B.git` and
  `https://github.com/a/b` resolve to the same project.
- A directory with a remote and no `--name` maps to the project keyed on
  `(user, remote)`. The first `run` creates it; every later `run` finds it.
- A `--name` project in a directory with no remote remembers the
  repository (or directory) it was created from (`projects.json` `by_dir`),
  so a later `run` there without `--name` finds it. That memory is only
  trusted while the project's remote matches the directory's (both empty
  for such a project), and naming a project explicitly (`repose attach
  izma`, `--project`) never writes it, so one checkout can never be sent
  to another checkout's guest (DECISIONS I-152).
- `repose run` in a directory with no remote and no `--name` exits 2 with
  the one line above. Any other command that finds no project exits 4
  (`No repose project here, and this directory has no git remote. Name
  one: ...`). It never creates a project named after the directory, because the
  directory name is not unique and the project would be unfindable from a
  second checkout.
- The project name becomes the slug: lowercase, `[a-z0-9-]`, other characters
  replaced by `-`, runs collapsed, 1 to 40 characters. `Todo App` and
  `todo-app` collide, and the CLI says so with the existing project's name.
- Nothing is ever written into the user's repository. No `.repose` file, no
  git config key, no hook. Evidence for a test: `git status` before and after
  `repose run` shows the same tree.
- `REPOSE_PROJECT` or `--project <id or slug>` overrides directory
  resolution for every command, so scripts can run `repose stop --project
  todo-app` from anywhere.

Limits:

- A user with no paid invoice yet may have 3 projects, at most 1 of class
  `xl`. After the first paid invoice, 10 projects. `POST /projects` past the
  limit returns `invalid` with the limit in `detail`, and the CLI prints
  `you have 3 of 3 projects; destroy one or add a card and pay your first
  invoice to raise the limit`.
- `repose fork` makes N projects at once and is refused whole, before any
  is created, when N more would pass the limit (snapshots.md, "Forking";
  DECISIONS I-254).
- The capacity waitlist (DECISIONS I-269). A user's first project waits
  when the fleet is near full: if the memory reserved on ready hosts, plus
  8 GB for each user admitted in the last 72 hours who has not created a
  project yet, plus the new project's class, would pass `WAITLIST_PERCENT`
  (default 80, the HostMemory80 line) of the hosts' usable memory, or when
  anyone is already waiting, `POST /projects` returns 503 `waitlisted` with
  `{position, joined_at, email}` and the user joins the queue; a retry
  keeps the place. The CLI prints `repose is at capacity. You're number N
  on the waitlist; we'll email you@example.com when there's room.` and
  exits 8. `GET /me` carries the place and the dashboard's empty projects
  page shows it. Every minute the api admits the queue oldest first while
  that projection, with each admission counted as a large, stays under the
  line, and sends one email per admission (transactional: it goes out even
  with notify email off). Users who have ever had a project, users once
  admitted and `exempt` accounts are never waitlisted; they can still meet
  plain `capacity`. `repose-admin waitlist list | admit HANDLE | admit
  --next N` lets an operator see and move the queue.
- A user without a card on file cannot start a guest at all
  (`payment_required`, exit 7). Creating the project row is allowed so the
  dashboard can show it, but nothing boots.
- `repose resize [PROJECT] [DISK]` grows the disk (`80G`; disks never shrink).
  PROJECT is positional like every other command's (I-155); a lone
  argument that parses as a size is DISK (I-268).
  `repose resize --size small|large|xl` changes the class (DECISIONS
  I-260): the API accepts `class` on `PATCH /projects/:id` only while the
  project is stopped (`conflict` otherwise), so a running project is
  stopped with a snapshot, patched and started again after a y/N question
  (`--yes` skips it; no terminal and no `--yes` is exit 2). The same class
  is a no-op; a refused PATCH (the xl limit) starts the project again at
  its old class. StartGuest carries the class on every start, so the
  guest boots at the new vCPUs and memory and its samples, and so its
  billing, report the new class. The dashboard does not change a class.

Ownership:

- A project belongs to exactly one user. There is no sharing, no transfer, no
  team in the first release.
- Destroying a project keeps its row for usage history and keeps the last
  snapshot 30 days (see stop-start-destroy.md).

## Depends on

Workstreams 05 (api projects routes, limits), 07 (cli resolution), 09 (limit
changes on first paid invoice).

## Deferred

Teams and shared projects (DECISIONS R5-6). Project transfer between users.
Renaming a project (the slug is baked into the tmux session name and the
SSH login; a rename is a destroy-and-restore until someone designs it).
