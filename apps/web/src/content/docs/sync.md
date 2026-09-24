---
title: What gets synced
description: What repose run copies to the machine, what it leaves behind, and how work comes back.
section: Using repose
order: 12
---

Each `repose run` copies your laptop's current state of the checkout to the machine, once, at the start. Nothing syncs continuously, and nothing ever syncs from the machine back to your laptop. Work comes back through git: the agent commits and pushes, you pull.

`repose attach` never touches the checkout. `repose run --no-sync` skips it too.

## Your code

All of it goes over the command's SSH connection to the machine. That connection runs through repose's SSH gateway, which relays it and stores none of it. None of your code or files go to repose's API or database.

**Commits.** Your current branch is created on the machine, or fast-forwarded if the machine's copy is behind. Commits you haven't pushed travel anyway; nothing is pushed for you. The machine gets them from your laptop directly, so it never needs access to your git host for this, and private repositories work without any setup.

**Uncommitted changes to tracked files** are carried as a diff and applied on the machine.

**Untracked files** that git isn't ignoring are copied as they are.

**`.env` files.** Gitignored files named `.env` or `.env.*`, anywhere in the checkout, up to 1 MB each, are copied too, with mode 0600. If the machine's copy is newer than the laptop's, the machine's is kept and the CLI tells you. A `.env` inside a directory that is ignored as a whole (say `build/.env` with `build/` in `.gitignore`) is not copied.

After the sync you'll see a summary:

```
Synced: 4 modified, 2 untracked, 2 env files (3 new commits)
```

It always prints, even when everything is zero.

## What never travels

- Other gitignored files. Build output, caches and local databases stay on the laptop.
- Dependency and cache directories, even when they aren't gitignored: `node_modules`, `.pnpm-store`, `.venv`, `venv`, `__pycache__`, `.next`, `.turbo`, `.svelte-kit` and others like them, at any depth. The CLI names each one it skipped (`Not sent: web/node_modules`). Install dependencies on the machine; they need to be built for Linux anyway.
- Any single file over 100 MB. The CLI warns and skips it.
- Untracked files past 500 MB in total for one sync. The rest are skipped with one warning.
- Directories and special files in the untracked list. Symlinks are copied as symlinks and never followed.

To keep more things off the machine, add patterns to `sync.exclude` in `~/.config/repose/config.toml`:

```toml
[sync]
exclude = ["dist", "*.mp4", "fixtures/large"]
```

The patterns use gitignore syntax. A directory name matches at any depth. With v0.1.9 and older, write it as a top-level quoted key instead: `"sync.exclude" = ["dist"]`.

## When the machine has changes of its own

Before copying anything, the CLI checks whether the machine's checkout has uncommitted changes. Changes left by your own previous sync don't count: the CLI recognises them, stashes them on the machine with the message `repose run: last sync`, and lays down your current ones. Only the ten newest of those stashes are kept.

Any other change on the machine (an agent's edit, a new file, a commit it made without pushing) stops the sync:

```
The guest's working tree has uncommitted changes (3 files):
  M src/auth.ts
  M src/routes/login.ts
  ?? notes.md
An agent may still be working. Re-run with --stash-remote (keeps them in `git stash`) or --discard-remote (throws them away), or `repose attach` to look first.
```

The command exits with code 6 and changes nothing. You have three choices:

- `repose attach` to look, and let the agent finish or commit.
- `repose run --stash-remote` to set the machine's changes aside with `git stash` (get them back with `git stash pop` on the machine).
- `repose run --discard-remote` to throw them away.

If an agent committed on the machine's branch and you haven't pulled those commits, the sync leaves that branch where it is and checks out your laptop's commit detached, with a warning. The agent's commits are never moved or lost. Push them from the machine (or `repose attach` to see them) and pull on your laptop.

## Getting work back

The machine's checkout has `origin` set to your repository's remote, and `origin/<branch>` points where your laptop last saw it, so `git status` and `git push` on the machine behave the way they do locally.

For pushing to work, the machine needs credentials for your git host. If you're logged in to the GitHub CLI (`gh`) on your laptop and the remote is on github.com, your `gh` login is copied over and git on the machine is set up to push over HTTPS with it. Nothing else is needed. For other hosts, see [Secrets and logins](/docs/secrets).

To see whether the machine's checkout has uncommitted work, attach and run `git status`.

To fetch a single file without going through git, see `repose cp` below.

## The first sync of a big repository

If the machine's checkout is empty, the remote is on github.com and the repository's packed history is 20 MB or more, the machine clones the history from GitHub itself (using your `gh` login for a private repository). Your laptop then sends only the commits GitHub doesn't have, plus your uncommitted work. The summary ends with `history cloned from github.com`. If that clone fails for any reason, the CLI sends the history from your laptop instead and says why. Later syncs never clone.

## Repositories the sync can't handle

The checkout has to be a git repository with at least one commit and full history. For each case the CLI prints what to run:

- Not a repository, or no commits yet: `git init && git add -A && git commit -m init`.
- A shallow clone: `git fetch --unshallow`.

Or pass `--no-sync` and put the code on the machine yourself.

## Your tools

> Not in a release yet. v0.1.9 and older don't do this.

`repose run` also makes sure the machine has the command-line tools you use. It reads which tools you've installed globally on your laptop (with npm, pnpm, bun, `go install`, `cargo install`, `uv tool` or pipx) and which commands your project's scripts call (in `package.json`, `Makefile`, `justfile`, `Procfile`, `.air.toml` and compose files). Only names and versions are sent. Tools the machine already has, and commands a project dependency provides, are skipped.

The machine installs the rest in the background and says so once:

```
Installing 3 of your tools in the background: air, portless, typescript
```

Nothing waits for these installs. If you run one of those commands before it's ready, the shell says the tool is on its way. A tool that fails to install is named at the next run, with the installer's last line; the full log is `~/.repose/tools-install.log` on the machine.

If your project pins a Node major version (`.nvmrc`, `.node-version`, `.tool-versions`, `volta.node`, or `engines.node` set to one major) that the machine lacks, that version is installed and made the default `node` for new shells.

To see the list without installing anything or contacting the machine:

```
repose scan
```

## `repose cp`

> Not in a release yet. v0.1.9 and older don't have this command.

For the odd file that shouldn't go through git, like a log, a trace or a fixture:

```
repose cp :logs/app.log .                # from this checkout's machine
repose cp todo-app:/tmp/trace.json .     # from any project
repose cp -r ./fixtures :test/fixtures   # laptop to machine
```

A path after `:` is on the machine. Relative paths start at the checkout, `/home/dev/<project>`. It's `scp` underneath, so the machine has to be running.
