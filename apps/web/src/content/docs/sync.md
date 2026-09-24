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
- **`.env` files.** Gitignored `.env` and `.env.*` files up to 1 MB each. If the machine's copy is newer, it's kept.

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

## When the machine has changes of its own

If something on the machine changed the checkout since your last sync (usually an agent), the sync stops instead of overwriting it:

```
The guest's working tree has uncommitted changes (3 files):
  M src/auth.ts
  M src/routes/login.ts
  ?? notes.md
```

Nothing is changed and the command exits with code 6. Then either:

- `repose attach` to look, and let the agent finish or commit;
- `repose run --stash-remote` to keep the machine's changes in `git stash` there;
- `repose run --discard-remote` to throw them away.

If the agent committed on the branch and you haven't pulled those commits, the sync checks out your laptop's commit detached and leaves the agent's branch where it is. Nothing is lost. Push from the machine and pull on your laptop.

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

For a large repository on github.com, the first sync has the machine clone the history from GitHub and sends only what GitHub doesn't have.
