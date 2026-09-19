# Sync at launch

Git is the exchange channel between the laptop and the guest. `repose run`
adds one thing on top: the uncommitted work in the laptop's tree is carried
over once, at launch, so the agent starts from what the user actually sees.
Work comes back only through git. Nothing syncs continuously.

## What the user sees

```
$ repose run
Syncing todo-app to a1b2c3d (main): fetching ... checking out ... done
Applying local changes: 3 modified, 1 untracked (12 KB) ... done
```

Local commit not on the remote yet:

```
$ repose run
Your HEAD (a1b2c3d) is not on origin. Push it now? [Y/n] y
Pushing main to origin ... done
```

Guest tree is dirty:

```
$ repose run
error: the guest's working tree has uncommitted changes (2 modified, 1 untracked):
  src/auth.ts  src/routes/login.ts  src/lib/session.ts
An agent may still be working. Choose:
  repose run --stash-remote     stash the guest's changes, then sync
  repose run --discard-remote   throw them away, then sync
  repose run --no-sync          attach without syncing
```

## Behaviour that must hold

- Sync runs only as part of `repose run`, never on `attach`, `start`, or
  any other command. `repose sync` exists as the same step on its own.
- Step one is the commit. The guest runs `git fetch origin` and checks out
  the laptop's `HEAD` commit by hash, on the laptop's branch name if it
  exists on the remote, detached otherwise. If the hash is not reachable
  from origin, the CLI offers to push the current branch; declining exits 1
  with nothing changed on either side. It never force-pushes.
- Step two is the diff. The CLI builds a tar stream of: every file `git diff
  HEAD --name-only` lists (modified, added, deleted are represented; deleted
  files are sent as a deletion list), plus every untracked file that
  `git ls-files --others --exclude-standard` lists. Extra exclusions from
  `sync.exclude` in `config.toml` apply on top. The stream goes over the
  SSH session's stdin to `repose-guest-sync apply` in the guest, which
  writes files, applies deletions, and sets executable bits.
- Size: over 50 MB the CLI stops and says which files are large and how to
  exclude them, because an accidental `node_modules` or a video is the usual
  cause and the user wants to know.
- Refuse-on-dirty: before writing anything, the guest checks `git status
  --porcelain` in `/home/dev/<slug>`. If it is non-empty, the CLI prints the
  block above and exits 6. Nothing is written. This rule exists because the
  common case for a dirty guest tree is an unattended agent mid-task, and
  silently overwriting its work is the exact thing the product promises not
  to do.
- `--stash-remote` runs `git stash push -u -m "repose sync <timestamp>"`
  in the guest first, then syncs. The stash name is printed so the user can
  find it.
- `--discard-remote` runs `git checkout -- . && git clean -fd` in the guest
  first. It asks for confirmation unless `--yes`.
- Files ignored by gitignore never travel in either direction. `.env` files
  that are gitignored therefore do not sync; that is deliberate and
  documented, and named secrets (secrets.md) are the supported path.
- An empty diff produces one line, `nothing to sync`, and takes under one
  second beyond the fetch.
- The guest's checkout is at `/home/dev/<slug>`. If the directory is not a
  git repository (project created with `--name` in a directory that was not
  one), the guest initialises it and adds the remote if there is one.
- After sync, `git status` in the guest shows exactly the laptop's
  uncommitted changes and nothing else. A test asserts that with a fixture
  of modified, added, deleted, untracked, and executable files.

Back to the laptop:

- There is no reverse sync. The user pulls. `repose status` shows the
  guest's branch, HEAD, and whether the tree is dirty so the user knows
  something is waiting to be committed.

## Depends on

Workstreams 07 (cli tar builder, prompts), 04 (`repose-guest-sync apply`
lives in the guest base and is called over SSH, not vsock), 02 (guest base
ships the helper).

## Deferred

`repose sync --watch` continuous sync (DECISIONS R1-3). Reverse sync of the
guest's uncommitted changes to the laptop. Syncing gitignored files by
explicit allowlist.
