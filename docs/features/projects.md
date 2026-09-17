# Projects

A project is one guest, its volume, its snapshots, its config, and its meter
rows, owned by one user. The CLI decides which project a command means from
the directory it runs in.

## What the user sees

```
$ cd ~/code/todo-app
$ factory run
Creating project todo-app (github.com/heracraft/todo-app) as large ...
```

```
$ cd ~/scratch/no-remote
$ factory run
error: this directory has no git remote. Give the project a name:
  factory run --name scratch
```

```
$ factory run --name todo-app-experiment
Creating project todo-app-experiment (github.com/heracraft/todo-app) ...
```

## Behaviour that must hold

Identity:

- The CLI reads `git remote get-url origin` and normalises it as
  `interfaces/cli-config.md` says. `git@github.com:A/B.git` and
  `https://github.com/a/b` resolve to the same project.
- A directory with a remote and no `--name` maps to the project keyed on
  `(user, remote)`. The first `run` creates it; every later `run` finds it.
- `--name` always wins over the remote. A `--name` project remembers the
  directory it was created from (`projects.json` `by_dir`), so a later `run`
  in that directory without `--name` finds it, and a `run` in the same
  directory with a different `--name` creates another project. Two projects
  can therefore share a remote when the user asked for it and never by
  accident.
- A directory with no remote and no `--name` exits 2 with the one-line fix
  above. It never creates a project named after the directory, because the
  directory name is not unique and the project would be unfindable from a
  second checkout.
- The project name becomes the slug: lowercase, `[a-z0-9-]`, other characters
  replaced by `-`, runs collapsed, 1 to 40 characters. `Todo App` and
  `todo-app` collide, and the CLI says so with the existing project's name.
- Nothing is ever written into the user's repository. No `.factory` file, no
  git config key, no hook. Evidence for a test: `git status` before and after
  `factory run` shows the same tree.
- `FACTORY_PROJECT` or `--project <id or slug>` overrides directory
  resolution for every command, so scripts can run `factory stop --project
  todo-app` from anywhere.

Limits:

- A user with no paid invoice yet may have 3 projects, at most 1 of class
  `xl`. After the first paid invoice, 10 projects. `POST /projects` past the
  limit returns `invalid` with the limit in `detail`, and the CLI prints
  `you have 3 of 3 projects; destroy one or add a card and pay your first
  invoice to raise the limit`.
- A user without a card on file cannot start a guest at all
  (`payment_required`, exit 7). Creating the project row is allowed so the
  dashboard can show it, but nothing boots.
- Changing a project's class requires it to be stopped. The CLI stops it with
  a prompt if asked to resize a running project.

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
