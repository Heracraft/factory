---
title: Sync
description: What repose run copies to the machine, what it leaves behind, and how work comes back.
section: Using repose
order: 11
---

Each `repose run` copies the current state of your checkout to the machine, once, at the start. Nothing syncs continuously and nothing comes back on its own: the agent commits and pushes, and you pull.

`repose attach` and `repose run --no-sync` leave the machine's checkout alone.

## What travels

- **Commits.** Your current branch, including commits you haven't pushed. They go straight from your laptop, so private repositories work with no setup on the machine.
- **Uncommitted changes** to tracked files.
- **Untracked files** that git isn't ignoring.
- **`.env` files.** Gitignored `.env` and `.env.*` files up to 1 MB each. If the machine's copy is newer, it's kept. Unlike [secrets](/docs/secrets), they are files on the machine's disk, so they are in snapshots.

Everything goes over your SSH connection. None of it is stored by repose.

```
Synced: 4 modified, 2 untracked, 2 env files (3 new commits)
```

## What doesn't

- Other gitignored files: build output, caches, local databases.
- Dependency directories such as `node_modules`, `.venv`, `.next` and `.turbo`, even when they aren't ignored. The CLI names what it skipped. Install dependencies on the machine; they need Linux builds anyway.
- Files over 100 MB, and untracked files past 500 MB in one sync.

To leave out more, add gitignore-style patterns to `~/.config/repose/config.toml`:

```toml
[sync]
exclude = ["dist", "*.mp4", "fixtures/large"]
```

## Submodules

Submodules you have checked out travel the same way, nested ones included. Each arrives at the commit your laptop has checked out in it, with its uncommitted changes, untracked files and `.env` files, under the same rules and limits as the rest of the checkout. Submodule commits you haven't pushed travel too, and a private submodule needs no access from the machine. A submodule you never checked out on your laptop stays empty on the machine.

A submodule that is a shallow clone on your laptop can't be sent. The machine fetches it from its own remote instead, which works for github.com when you're logged in to `gh`. If that fails, the run goes on and the CLI says the submodule is empty on the machine. Changes inside a shallow submodule aren't sent; run `git -C <path> fetch --unshallow` on your laptop to send them.

Git LFS files arrive as their small pointer files, not their contents. Run `repose config add git-lfs` once, then `git lfs pull` on the machine to fetch them.

## When the machine has changes of its own

`repose run` only copies your laptop's work onto the machine. It never restarts or rebuilds the machine, so running it again on a machine an agent is working on is safe.

If the machine changed since your last sync (usually an agent's edits or commits) and your laptop has nothing new since then, there is nothing to copy. The checkout is left as it is and you're attached:

```
The machine has changes your laptop doesn't have (27 files); attaching without syncing. `repose run --stash-remote` puts them in git stash and syncs your laptop's work.
```

If your laptop does have new work, copying it would write over the machine's changes, so the sync stops, changes nothing and exits with code 6:

```
`repose run` copies your laptop's work onto the machine. It doesn't restart or rebuild anything.
The machine has uncommitted changes your laptop doesn't have (27 files), probably an agent's:
  src/auth.ts
  src/routes/login.ts
  src/routes/logout.ts
  src/session.ts
  src/session.test.ts
  package.json
  pnpm-lock.yaml
  notes.md
  and 19 more
Your laptop has new work as well, so syncing now would write over them. Nothing was changed. Pick one:
  repose attach                  look at the machine first
  repose run --stash-remote      put the machine's changes in git stash, then sync
  repose run --discard-remote    throw the machine's changes away, then sync
```

`--stash-remote` keeps the machine's changes in `git stash` there, named `repose run`. `--discard-remote` throws them away.

Changes that are exactly what the previous sync wrote don't count as the machine's: they are stashed on the machine as `repose run: last sync` (the newest 10 are kept) and the sync goes on.

If the agent committed on the branch and your laptop has new commits of its own, the sync checks out your laptop's commit detached and leaves the agent's branch where it is. Nothing is lost. Push from the machine and pull on your laptop.

## Getting work back

The machine's checkout has the same `origin` as yours, so `git push` there works as it does locally. If you're logged in to the GitHub CLI (`gh`) on your laptop and the remote is on github.com, that login is copied and git on the machine pushes over HTTPS with it. For other git hosts, see [Secrets](/docs/secrets#other-git-hosts).

For a file that shouldn't go through git, use `repose cp`. A path after `:` is on the machine, relative to the checkout:

```
repose cp :logs/app.log .
repose cp todo-app:/tmp/trace.json .
repose cp -r ./fixtures :test/fixtures
```

## Repositories the sync can't handle

The checkout must be a git repository with at least one commit and full history. The CLI tells you what to run:

- No repository or no commits: `git init && git add -A && git commit -m init`
- A shallow clone: `git fetch --unshallow`

A directory without a remote needs a name the first time: `repose run --name scratch`.

For a repository on github.com over about 20 MB, the first sync has the machine clone the history from GitHub and sends only what GitHub doesn't have. If that clone fails, the CLI sends everything itself.
