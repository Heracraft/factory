# Sync at launch

Git is the exchange channel between the laptop and the guest. `repose run`
adds one thing on top: the uncommitted work in the laptop's tree is carried
over once, at launch, so the agent starts from what the user actually sees.
Work comes back only through git. Nothing syncs continuously.

## What the user sees

```
$ repose run
Connected to todo-app (large)
Synced: 3 modified, 1 untracked
```

Local commit not on the remote yet:

```
$ repose run
Commit a1b2c3d is not on origin. Push main now? [Y/n] y
Connected to todo-app (large)
Synced: 0 modified, 0 untracked
```

Guest tree is dirty:

```
$ repose run
The guest's working tree has uncommitted changes (3 files):
  M src/auth.ts
  M src/routes/login.ts
  ?? notes.md
An agent may still be working. Re-run with --stash-remote (keeps them in
`git stash`) or --discard-remote (throws them away), or `repose attach`
to look first.
```

## Behaviour that must hold

- Sync runs only as part of `repose run`, never on `attach`, `start`, or
  any other command. There is no standalone `repose sync`.
- The guest checks `git status --porcelain` in `/home/dev/<slug>` first
  (this includes untracked files, so an agent's scratch file counts as
  dirty too). If it is non-empty and neither `--stash-remote` nor
  `--discard-remote` was given, the CLI prints the block above and exits
  6; nothing else in the sync step runs. `--stash-remote` runs `git stash
  push -u -m "repose run"` in the guest first; `--discard-remote` runs
  `git reset --hard && git clean -fd`. Neither asks for confirmation —
  the flag itself is the confirmation.
- Then the guest runs `git fetch origin` and checks whether the laptop's
  `HEAD` commit is reachable (`git cat-file -e`). If not, the CLI offers
  to push the current branch (`Commit <hash> is not on origin. Push
  <branch> now? [Y/n]`); declining fails the run with nothing changed on
  either side. It never force-pushes.
- The guest then checks out that commit detached, and rides the local
  branch name instead only if that branch already sits at the same commit
  (a `git fetch` moves remote-tracking refs, never local branches, so a
  stale local branch is left alone rather than silently overwriting the
  detached checkout).
- The diff, not a tar, carries tracked changes: `git diff HEAD --binary`
  on the laptop, piped as stdin to `git apply --index` in the guest. Only
  the untracked files (`git ls-files --others --exclude-standard`,
  filtered by `sync.exclude` in `config.toml`) travel as a tar, extracted
  with `tar -x -C ~/<slug>`. Nothing writes a custom sync helper into the
  guest; both commands are stock git and tar.
- Size: a file over 100 MB is skipped with a warning rather than sent,
  because an accidental `node_modules` or a video is the usual cause and
  the user wants to know.
- Files ignored by gitignore never travel in either direction. `.env`
  files that are gitignored therefore do not sync; that is deliberate and
  documented, and named secrets (secrets.md) are the supported path.
- The summary line always prints, `Synced: <n> modified, <m> untracked`,
  even when both are zero.
- The guest's checkout is at `/home/dev/<slug>` and is assumed to already
  exist and be a clone with an `origin` remote (guestd's `SetupProject`,
  02/04's contract); the CLI does not initialise a repository there.

Back to the laptop:

- There is no reverse sync. The user pulls. `repose status` shows the
  guest's branch, HEAD, and whether the tree is dirty so the user knows
  something is waiting to be committed.

## Depends on

Workstreams 07 (cli: the diff/tar builder and the git commands, all run
over the SSH session, never vsock), 04/02 (guestd's `SetupProject`
guarantees the guest checkout exists before the CLI's first sync).

## Deferred

`repose sync --watch` continuous sync (DECISIONS R1-3). Reverse sync of the
guest's uncommitted changes to the laptop. Syncing gitignored files by
explicit allowlist.
