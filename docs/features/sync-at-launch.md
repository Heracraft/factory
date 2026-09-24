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

Local commits not on the remote yet travel anyway, and nothing is pushed:

```
$ repose run
Connected to todo-app (large)
Synced: 0 modified, 0 untracked (2 new commits)
```

An agent committed on the guest's branch and the laptop has not pulled:

```
$ repose run
Connected to todo-app (large)
Synced: 1 modified, 0 untracked (1 new commit)
The guest's main has commits your laptop does not have; it was left as it
is and the guest is on 4f2a9c1, detached. Push them from the guest (or
`repose attach` to look) and pull on the laptop.
```

Guest changed since the last sync, laptop has nothing new (DECISIONS
I-248):

```
$ repose run
The machine has changes your laptop doesn't have (27 files); attaching without syncing. `repose run --stash-remote` puts them in git stash and syncs your laptop's work.
```

Guest changed and the laptop has new work that would land on it:

```
$ repose run
`repose run` copies your laptop's work onto the machine. It doesn't restart or rebuild anything.
The machine has uncommitted changes your laptop doesn't have (27 files), probably an agent's:
  src/auth.ts
  src/routes/login.ts
  ...(eight names in all)
  and 19 more
Your laptop has new work as well, so syncing now would write over them. Nothing was changed. Pick one:
  repose attach                  look at the machine first
  repose run --stash-remote      put the machine's changes in git stash, then sync
  repose run --discard-remote    throw the machine's changes away, then sync
```

## Behaviour that must hold

- Sync runs only as part of `repose run`, never on `attach`, `start`, or
  any other command. There is no standalone `repose sync`.
- The guest checks `git status --porcelain` in `/home/dev/<slug>` first
  (this includes untracked files, so an agent's scratch file counts as
  dirty too). If it is non-empty (and not the last sync's own, below)
  and neither `--stash-remote` nor `--discard-remote` was given, what
  happens depends on the laptop (DECISIONS I-248). When the laptop has
  nothing new since the sync the guest last took (its sync key equals the
  guest's `.git/repose-synced-key` and the guest has every commit it
  would send), there is nothing to write over: the checkout is left
  alone, only the logins and carry go, and the run attaches with the
  one-line notice above. The same holds when the guest's tree is clean
  but it moved on (an agent's commits, another branch): no detached
  checkout of an older laptop commit. Otherwise the CLI prints the
  refusal above (eight names, then a count) and exits 6; nothing else in
  the sync step runs. `--stash-remote` runs `git stash
  push -u -m "repose run"` in the guest first; `--discard-remote` runs
  `git reset --hard && git clean -fd`. Neither asks for confirmation —
  the flag itself is the confirmation.
- A run with nothing new applies nothing (DECISIONS I-224): when the
  laptop would send exactly what the last completed sync sent (the same
  commit, branch, diff and untracked files) and the guest's tree is still
  what that sync left, the apply is skipped, nothing is stashed, and the
  summary line ends "the guest already had them".
- The laptop's own changes are not an agent's (DECISIONS I-210). A sync
  that carried a modified or untracked file leaves the guest's tree dirty
  by construction, so the apply records a fingerprint of the tree it left
  (`HEAD` plus the tree `git add -A` would write, in the checkout's
  `.git/repose-synced`). When the next probe finds the tree dirty and the
  fingerprint unchanged, the run goes on: the apply stashes those
  changes (`git stash push -u -m "repose run: last sync"`, so nothing
  misjudged is lost; only the newest 10 such stashes are kept, and the
  user's own stashes and `--stash-remote`'s are never dropped), lays down the laptop's current ones, and the summary
  line ends "the last sync's changes stashed in the guest". An edit to a
  synced file, a new file, a commit, or any change inside a submodule
  changes the fingerprint and refuses as above, including one made
  between the probe and the apply; a tree that was clean at the probe is
  checked again before the files are laid down, so an agent's new file is
  never overwritten by one the laptop sends. `git stash push -u` cleans
  the untracked files after recording them, so a file written in that
  instant is lost (git's own behaviour). The checks force
  `status.showUntrackedFiles=normal` and `submodule.recurse=false`, so a
  carried laptop setting cannot hide a file or reach into a submodule.
  Both ride the two existing ssh round trips. A run from a second laptop
  sees the first laptop's synced changes as the last sync's own too, and
  stashes them in the guest the same way before laying down its own.
- The guest never fetches from origin during a sync: it has no
  credentials for a private repository, nor for a public one behind the
  SSH `origin` guestd sets (DECISIONS I-150). The laptop sends the commits
  itself. The first ssh (the same one that reads the dirty list) reports
  every commit a ref in the guest points at; the laptop `git bundle
  create`s `HEAD` and its own `origin/<branch>` minus what the guest has
  (the whole history the first time, nothing when the guest is current),
  and the second ssh fetches from that bundle. Nothing is pushed, and the
  old "Push now?" prompt is gone: an unpushed commit simply travels.
- The guest's `origin/<branch>` is moved to where the laptop last saw
  origin (forward only), and `origin` is added if missing, so the agent's
  `git status` and `git push` behave as they would on the laptop.
- Checkout: the laptop's branch is created in the guest, or
  fast-forwarded when the guest's copy is behind. When the guest's branch
  has commits the laptop does not (an agent committed and nobody pulled),
  the branch is left exactly where it is, the laptop's commit is checked
  out detached, and a warning says so; an agent's work is never moved off
  its branch. A detached `HEAD` on the laptop is checked out detached.
- The diff, not a tar, carries tracked changes: `git diff HEAD --binary`
  on the laptop, applied with `git apply --index` in the guest. Only
  the untracked files (`git ls-files --others --exclude-standard`,
  filtered by `sync.exclude` in `config.toml`) travel as a tar, extracted
  in `~/<slug>`. Bundle, diff and untracked tar go as one payload in one
  ssh; nothing writes a custom sync helper into the guest, and every
  command there is stock git and tar.
- An empty guest checkout (a bare `git init`, or no directory at all) is
  filled the same way: the first bundle is the full history.
- A laptop directory that is not a git repository, has no commit yet, or
  is a shallow clone gets one sentence saying what to run
  (`git init && git add -A && git commit -m init`, `git fetch
  --unshallow`) or `--no-sync`.
- Size: a file over 100 MB is skipped with a warning rather than sent,
  because an accidental video is the usual cause and the user wants to
  know; past 500 MB of untracked files in one sync the rest is skipped
  with one warning (DECISIONS I-194).
- Dependency and cache directories never travel, gitignored or not:
  `node_modules`, `.pnpm-store`, `.venv`, `venv`, `__pycache__`, `.next`,
  `.turbo`, `.svelte-kit` and the like, wherever they sit in the tree. The
  sync names each one it left behind once (`Not sent: cms/node_modules`);
  the agent installs dependencies in the guest, where they are built for
  the guest's platform. A `sync.exclude` pattern excludes a directory at
  any depth (`dist` covers `web/dist/...`).
- Symlinks travel as symlinks, never followed; directories and special
  files in the untracked list are skipped, and one unreadable file does
  not stop the rest.
- Files ignored by gitignore do not travel, with one exception (DECISIONS
  I-197, which reverses this document's earlier "never"): gitignored
  `.env` and `.env.*` files at any depth outside the dependency
  directories above, up to 1 MB each, go laptop to guest in the sync's
  own apply ssh, after the checkout (so the guest's `.gitignore` already
  covers them), mode 0600, never through the api. A guest copy newer than
  the laptop's is kept, and the CLI says so once: `Kept the guest's
  apps/web/.env: it is newer than the laptop's.` An unchanged set is not
  sent again (marker `env`, I-206). They sit on the guest disk and so are
  in snapshots, like the gh token and the code. Named secrets
  (secrets.md) remain the path for values that must change without a
  laptop. Ignored directories are listed collapsed (`git ls-files
  --others --ignored --exclude-standard --directory`), so a `.env`
  inside a wholly ignored directory does not travel. Contents never reach
  a log line.
- The summary line always prints, `Synced: <n> modified, <m> untracked`,
  even when both are zero, followed by `, <e> env files` when .env files
  were written and `(<k> new commits)` when commits travelled.
- The guest's checkout is at `/home/dev/<slug>`, which guestd's
  `SetupProject` creates with an `origin` (02/04's contract). If it is
  missing anyway, the sync creates it (`git init`) and adds `origin`
  rather than failing.
- The first sync of a large GitHub repository clones in the guest
  (DECISIONS I-203). When the guest has no commits yet, the remote is on
  github.com and the laptop's `git count-objects -v` reports a
  `size-pack` of 20 MB or more (the starting threshold, to be set from
  measurement), the guest fetches every branch and tag from
  `https://github.com/<owner>/<repo>.git` itself, after the credentials
  step (so a private repository uses gh's login when it travelled; a
  public one needs none). The laptop then bundles only the commits GitHub
  lacks, and the diff, untracked and `.env` files follow as always. The
  summary line ends `, history cloned from github.com`. A clone that
  fails for any reason falls back to the full bundle, with `The guest
  could not clone from GitHub (<git's reason>), so the history was sent
  from your laptop instead.`; the run never fails for it. A full fetch,
  never a partial clone: lazily fetched blobs fail later once a token
  expires. Later syncs never clone.
- A project made with `--name` in a directory with no git remote syncs the
  same way, without an `origin` or remote-tracking refs: its real commits
  travel, so a file deleted and committed on the laptop is deleted in the
  guest too (DECISIONS I-150, replacing I-138's whole-tree commit).
- Credentials (secrets.md) are copied before the git steps, in one ssh.
  When gh's login travelled and the remote is on github.com, the guest's
  git is told to push to github over HTTPS with gh as the credential
  helper (`guest-conventions.md`), so an agent's `git push` works without
  the laptop's SSH keys.
- All of this rides the command's one multiplexed SSH connection (I-149):
  three round trips in all: the probe (which also returns the carry's
  markers), the credentials and carry, and the apply (with the .env
  files).

Tools (DECISIONS I-221, I-222):

- `repose run` makes the guest have the tools the laptop has and the
  project runs. It lists the laptop's global tools by reading the
  managers' install directories, never by running them: npm's global
  prefix (`$NPM_CONFIG_PREFIX`, `prefix=` in `~/.npmrc`, else the
  directory above `node`'s), pnpm's (`$PNPM_HOME/global`), bun's
  (`~/.bun/install/global`), Go binaries in `$GOBIN`, `$GOPATH/bin` or
  `~/go/bin` (package path and version from their build info), cargo's
  `~/.cargo/.crates2.json` (registry crates only), `uv tool` and `pipx`
  venvs. It adds the commands the checkout's own scripts run: package.json
  scripts at the root and in every workspace package, Makefile and
  justfile recipes, Procfile, `.air.toml` (air) and compose files
  (docker). A command is left out when the guest base has it, when a
  dependency of that workspace or the root provides it (by name, a known
  package-to-command table, or `node_modules/.bin`), when a workspace
  package's `bin` or a pyproject script defines it, or when it is the
  name of another script. `npx`, `pnpm dlx` and `uv run` fetch what they
  run and name nothing.
- What travels is names, managers, versions and Go package paths, nothing
  else: no laptop path, no config value. It rides the carry (marker
  `tools`), so an unchanged list costs nothing, and the laptop's reading
  runs while the probe's ssh is in flight (about 0.5 ms measured on the dev box
  against nuru-playground; the budget is 20 ms).
- The guest answers in milliseconds with what it lacks, and the CLI says
  it once: `Installing 3 of your tools in the background: air, portless,
  typescript`. The installs run after that in a low-priority user unit
  (`repose-tools-carry`); nothing in `run` waits for them. Each tool is
  installed from nixpkgs when a package there has `bin/<command>` (a
  prebuilt binary, pinned into the store overlay with the rest of dev's
  profile), else with the laptop's manager into a directory on the login
  PATH (`npm i -g`, `go install` with `GOBIN=~/.local/bin`, `cargo
  install --root ~/.local`, `uv tool install`). A command only a project
  script names is installed from nixpkgs or, for the few npm-only ones
  (`portless`), from npm. While a tool installs, its commands are listed
  in `$XDG_RUNTIME_DIR/repose-installing`, so a shell that runs one early
  says it is on its way instead of "command not found".
- A project that pins a node major (`.nvmrc`, `.node-version`,
  `.tool-versions`, `volta.node`, or an `engines.node` of one major such
  as `22`, `22.x`, `^22.1`) the guest does not have gets `nodejs_<major>`
  in dev's nix profile, but only when that makes it the `node` of new
  shells, which it checks. When a `node` earlier on PATH would still win,
  nothing is installed and the CLI says to run `repose config add
  nodejs_<major>`. Go, Rust and Python version files are left to
  `GOTOOLCHAIN`, rustup and uv, which fetch the version themselves.
- A tool that could not be installed is named once, at the next `run`:
  `Could not install air: <the installer's last line>`. The full output
  is in the guest's `~/.repose/tools-install.log`. A failed tool is not
  retried until the laptop's entry for it changes.
- `repose scan [DIR]` prints the same list and why, without installing
  anything or contacting the guest, to check a project before a run.

Back to the laptop:

- `repose cp` copies a file either way when a log or a trace is needed
  for triage (DECISIONS I-201): `repose cp :logs/x.log .` from the
  checkout, `repose cp izma:/tmp/trace.json .` from anywhere, `repose cp
  -r ./fixtures :test/fixtures` the other way. It is scp over the
  project's connection with the project resolved as every other command
  resolves it, and guest paths relative to the checkout.
- There is no reverse sync. The user pulls. `repose status` shows the
  guest's branch, HEAD, and whether the tree is dirty so the user knows
  something is waiting to be committed.

## Depends on

Workstreams 07 (cli: the diff/tar builder and the git commands, all run
over the SSH session, never vsock), 04/02 (guestd's `SetupProject`
guarantees the guest checkout exists before the CLI's first sync).

## Deferred

`repose sync --watch` continuous sync (DECISIONS R1-3). Reverse sync of the
guest's uncommitted changes to the laptop. Syncing other gitignored files
by explicit allowlist.
