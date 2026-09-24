---
title: Projects
description: How a checkout becomes a project, naming, sizes and limits.
section: Using repose
order: 10
---

A project is one machine plus its disk, snapshots, secrets and configuration. Each project belongs to one account. There is no sharing between accounts yet.

## Which project a command means

The CLI works out the project from the directory you run it in. It reads `git remote get-url origin` and looks for a project with that remote. The URL is normalised first, so `git@github.com:Acme/Api.git` and `https://github.com/acme/api` count as the same repository.

The first `repose run` in a checkout creates the project. Every later command in any checkout of the same repository finds it, on any laptop you're logged in on.

To act on a project from somewhere else, name it. Most commands take the project as their argument:

```
repose attach todo-app
repose stop todo-app
repose logs todo-app -f
```

Any command also accepts `--project NAME` (or the project's id), and the `REPOSE_PROJECT` environment variable does the same thing for scripts:

```
REPOSE_PROJECT=todo-app repose status
```

`repose run` is the exception: its argument is the prompt for the agent, so use `repose run --project todo-app` to run against a project from outside its checkout. If the first word of a prompt is the name of one of your projects, `run` refuses and tells you which of the two you probably meant.

## Directories with no remote

A directory without an `origin` remote needs a name:

```
$ repose run
This directory has no git remote. Pass --name NAME to create a project anyway.

$ repose run --name scratch
```

The CLI remembers which directory `scratch` was created from, so later commands in that directory find it without `--name`. The directory still has to be a git repository with at least one commit, since the sync works through git. For a folder that isn't one yet:

```
git init && git add -A && git commit -m init
```

## A second project for the same repository

`--name` also works in a checkout that has a remote. This gives you a separate machine for the same repository, useful for trying something risky without touching your main project:

```
repose run --name todo-app-experiment
```

Later commands in that checkout still resolve to the original project. Reach the second one by name: `repose attach todo-app-experiment`.

## Names

A project's name becomes its slug: lowercase letters, digits and dashes, at most 40 characters. The slug appears in the tmux session name, the machine's hostname, its SSH alias (`todo-app.repose`) and the checkout path (`/home/dev/todo-app`).

If the name is already taken in your account, the CLI tries `name-2`, then `name-3`, and tells you which it picked.

A project can't be renamed. To change the name, destroy it and restore it under the new one with `repose restore OLD --as NEW`.

## Sizes

| Size    | vCPU | Memory | Disk  |
| ------- | ---- | ------ | ----- |
| `small` | 2    | 4 GB   | 20 GB |
| `large` | 4    | 8 GB   | 40 GB |
| `xl`    | 8    | 16 GB  | 80 GB |

New projects are `large` unless you choose otherwise. Pick the size when the project is created:

```
repose run --size xl
```

`--size` only applies at creation. Changing the size of an existing project isn't available in the CLI or the dashboard yet. To move to another size today, destroy the project and create it again with the size you want (commit and push first). To change the default for new projects, set `default_class` in [config.toml](/docs/cli-config).

The disk can grow without recreating anything. On the project's page in the dashboard, choose **Resize…** under Disk, pick a larger size and **Grow**. Disks can't shrink.

## Limits

An account that hasn't paid an invoice yet can have 3 projects, at most 1 of them `xl`. After the first paid invoice the limit is 10. Destroyed projects don't count, and a destroyed project's slot frees up as soon as the destroy starts.

## Listing projects

```
$ repose projects
PROJECT    CLASS  STATE    UP     AGENTS           TODAY  MONTH
todo-app   large  running  2h14m  claude: working  $0.31  $18.40
api-v2     xl     stopped  -      -                $0.00  $41.02
```

A project in `error` gets an extra line underneath saying why and what fixes it. `repose projects --json` prints the full records. `repose projects --destroyed` lists projects you destroyed in the last 30 days that can still be restored.
