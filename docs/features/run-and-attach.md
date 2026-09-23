# Run and attach

`repose run` is the whole product in one command. With no arguments it puts
you inside your project's tmux session. With a prompt it starts an agent in a
new tmux window, types the prompt, and leaves it running whether or not you
stay attached.

## What the user sees

First run on a new project (stderr shows a spinner with the elapsed time
on a terminal; this is what stays on screen, DECISIONS I-154):

```
$ repose run
✓ Created todo-app (large)  0.3s
nix › building '/nix/store/…-repose-guest.drv'...
✓ Built the environment  41s
✓ Booted todo-app  6.2s
Connected to todo-app (large)
Synced: 3 modified, 1 untracked (12 new commits)
Credentials: gh, git
Ready in 49s.
dev@todo-app:~/todo-app$
```

Without a terminal on stderr (CI, a pipe) the phases are plain lines,
`Creating todo-app...`, `Building the environment...`, `Booting
todo-app...`, `Connecting to todo-app...`, `Syncing...`. One `repose run`
makes one SSH connection and asks for nothing: the CLI's own key has no
passphrase (I-149).

Run with a prompt:

```
$ repose run "finish the auth flow, run the tests, commit when green"
Connected to todo-app (large)
Synced: 0 modified, 0 untracked
```

Run with another agent and an explicit size for a new project:

```
$ repose run --agent codex --size xl "port the build to bun"
```

Attach to an existing session later, from the checkout or from anywhere
by name (the project is the argument, DECISIONS I-155; `--project` works
too):

```
$ repose attach
$ repose attach izma
```

Attach to a project that is not running says which state it is in and
what to do, exit 5:

```
$ repose attach age-calculator
age-calculator is in an error state: the environment's agent (guestd) stopped answering; `repose start` restarts it.
```

`repose run izma` is refused with exit 2 when `izma` is one of your
projects: `run`'s argument is the prompt, so the CLI points at `repose run
--project izma` or `repose attach izma` instead of typing the word into
an agent (`--agent` sends it anyway). The prompt is everything after the
flags, so quoting is optional.

Run a prompt while an agent is already running:

```
$ repose run "also update the README"
warning: claude is already running in todo-app:claude on the same working tree.
Starting a second claude in window todo-app:claude-2. Two agents on one tree
can conflict; use `git worktree` inside the guest if that matters.
```

Running against a stopped project starts it first (the `Starting
todo-app` phase on stderr) before the usual `Connected to todo-app
(large)` line — there is no separate "stopped" message for `run` (only
`attach` and `open` refuse a guest that is not running, exit 5, since
starting one is not their job). A project in `error` is restarted by the
api on the same start (`Restarting todo-app (its agent stopped
answering)`, I-157).

## Behaviour that must hold

Session and windows (see `interfaces/guest-conventions.md`):

- The tmux session is named after the project slug and exists from guest
  boot, with a window `shell` whose working directory is
  `/home/dev/<slug>`. `repose run` with no prompt attaches to the session's
  current window.
- A prompt opens a window named after the agent (`claude`, `opencode`,
  `codex`, `gemini`, `pi`). If that window already exists, the new one is
  `<agent>-2`, then `-3`. The agent's interactive TUI runs in that window,
  never a headless or print mode, because the point is that the user can
  attach and see the live session with its history.
- The prompt is typed into the TUI only once the TUI is up. The CLI polls
  `tmux display -p '#{pane_current_command}'` and `tmux capture-pane`
  until the pane's process matches the agent and its content has been
  unchanged for 1 second, then `tmux send-keys -l '<prompt>'` followed by
  a separate `Enter`, exactly as 07-cli.md §5.5 step 7 specifies (a single
  `-l` send, not a bracketed paste).
- After starting the agent the CLI attaches to that window unless
  `--no-attach` was given. Detaching (`C-b d`) never stops anything.
- `repose attach` attaches to the project's current window; there is no
  `--window` flag (07-cli.md's command tree has none). If the session does
  not exist, `repose attach` fails the way any other tmux target failure
  does; recreating a lost session is guestd's job at boot
  (`guest-conventions.md`), not something the CLI drives.
- Two `repose run` invocations on the same project from two terminals both
  attach; tmux handles the multi-client case and the smaller terminal
  constrains the size, as tmux always does. That is documented, not hidden.

Agent picker:

- `--agent` accepts exactly `claude`, `opencode`, `codex`, `gemini`, `pi`.
  Anything else exits 2 listing the five. The default is the project's
  `agent_default`, which starts as `claude` and can be changed with
  `repose config` or the dashboard.
- The wrapper for the chosen agent installs its hooks (see agents.md) before
  exec. A prompt for an agent whose login is missing is handled as
  agents.md describes: the TUI's own login prompt appears in the window and
  the CLI tells the user to complete it.

Sequence and idempotency (from DESIGN §10):

1. Resolve or create the project (projects.md).
2. Ensure the guest is running; stream a pending build; start a stopped
   guest. An op that ends in `error` prints it and exits 1 (a build's own
   `eval_failed`/`build_failed` exits 10 instead, per the failure table
   below).
3. Get or refresh the SSH certificate; write the SSH config block and
   check the alias resolves (I-151).
4. Sync credential files (secrets.md), then the checkout
   (sync-at-launch.md). Both skipped with `--no-sync`.
5. Start the agent window if a prompt was given, then attach.

Running `repose run` twice in a row attaches twice and changes nothing else.
A test asserts that the second run makes no `POST` to the API except the
certificate refresh, if due.

Timing that must hold on a healthy host:

- A stopped guest starts and is attachable in under 10 seconds.
- A new project with an empty fragment is attachable in under 60 seconds,
  because the base closure is already in the host store.

Failure output:

- Not logged in: exit 3, `Not logged in. Run \`repose login\`.`
- No card: exit 7, `Add a card at https://repose.herakraft.co/billing
  first.`
- Host capacity exhausted: exit 8, `No capacity right now; try again in a
  few minutes. (We have been alerted.)` The API also raises a capacity
  alert.
- Build failed: exit 10, the Nix error verbatim, the fragment line if known,
  and `edit with \`repose config edit\``.
- SSH does not answer within 60s of the API reporting `running`: exit 1,
  `Guest is running but SSH did not answer in 60s. \`repose logs --kind
  console\` may show why.`, followed by ssh's last error line. A gateway
  refusal (`Permission denied`, a revoked certificate) during that window
  re-issues the certificate once and keeps waiting (07-cli.md §6,
  DECISIONS I-149).
- Any failed step in the guest: exit 1, `Could not <step>: <why> (<ssh's
  last line>).`, never the raw remote command (I-153).

## Depends on

Workstreams 07 (cli), 05 (projects, certs, ops), 04 (guestd SetupProject,
tmux control, send-keys idle wait), 02 (guest base, wrappers), 06 (gateway),
12 (build streaming).

## Deferred

`repose run --worktree` to start a second agent in a git worktree
automatically (DECISIONS R4-10 chose warn-and-proceed). Queueing prompts for
when the current agent finishes. Web terminal in the dashboard (R4-18).
