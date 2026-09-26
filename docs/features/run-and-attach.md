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
Another claude window is open; two agents share one working tree. `repose run --worktree` gives the next one its own.
```

The window is `claude-2`, then `claude-3`, and so on: the lowest free
number, with no limit (DECISIONS I-253).

Run a prompt in its own git worktree (I-253):

```
$ repose run --worktree "try the other approach"
Worktree: ~/todo-app-claude-2 on branch repose/claude-2
The worktree starts at the last commit; the uncommitted changes in ~/todo-app are not in it.
```

The second line appears only when the guest's checkout is dirty.
`--worktree` needs a prompt (exit 2 without one) and works for the first
window too (`~/todo-app-claude`, `repose/claude`). A guest checkout with
no `.git` or no commit is refused with exit 2. Each `--worktree` run makes
a new worktree; a later run never reuses one, and nothing removes them
but the user (`git worktree remove ~/todo-app-claude-2 && git branch -D
repose/claude-2`, documented in /docs/run-and-attach).

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
  the lowest free `<agent>-N` (`-2`, `-3`, ... with no limit, I-253). With
  `--worktree` the window opens in `~/<slug>-<window>`, a git worktree on
  branch `repose/<window>` (guest-conventions "tmux"); otherwise in the
  checkout. The agent's interactive TUI runs in that window,
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

The laptop's clock (I-198):

- Every `run` and `attach` moves the guest to the laptop's IANA zone when
  it differs: `/etc/repose/env` (read by every login shell) and tmux's
  global and per-session `TZ`, so a window or agent opened afterwards
  runs in the laptop's time, and the project's stored `tz` (`PATCH
  /projects/:id`), so the next boot starts in it too. Shells already
  running keep the zone they started with. When the zone changed, `run`
  prints `Time zone set to Asia/Tokyo.` on stderr; `attach` shows the same
  line inside tmux.
- Nothing here waits: on `run` it rides the SSH that copies the tool
  logins, on `attach` it runs beside tmux (the session helper, see
  below), and the api update runs beside the connect.
- A laptop whose zone cannot be named (no `TZ`, no zoneinfo link) leaves
  the guest's zone alone.

The session helper: `run` and `attach` start a small background process
(`repose __session`, detached, its options in the environment rather than
on its command line) just before the CLI becomes `ssh`. It does what must
not delay the attach and reports only through `tmux display-message`,
never over the pane, and it ends when the SSH session it was started
beside ends. Windows has no helper. While it runs it also keeps the
guest's listening ports forwarded to the laptop (ports-and-previews.md,
I-199), shown in the session's status bar.

Agent picker:

- `--agent` accepts exactly `claude`, `opencode`, `codex`, `gemini`, `pi`.
  Anything else exits 2 listing the five. The default is the project's
  `agent_default`, set at creation from `default_agent` in the laptop's
  `~/.config/repose/config.toml` (`claude` unless set; I-241). An existing
  project keeps its own; the API accepts `agent_default` on `PATCH
  /projects/:id`, but no CLI command or dashboard control changes it.
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

## Paste an image (I-252)

Claude Code reads a pasted image from the clipboard of the machine it
runs on, so `Ctrl-V` in the guest never sees the laptop's screenshot.
`repose paste [PROJECT] [--window NAME] [--print]` carries it across:

- The CLI reads a PNG from the laptop's clipboard: `pngpaste -`, else
  `osascript` writing `«class PNGf»` to a temp file, on macOS;
  `wl-paste --type image/png` when `WAYLAND_DISPLAY` is set, else
  `xclip -selection clipboard -t image/png -o` when `DISPLAY` is, on
  Linux (the offered types are listed first, so "no image" and "tool
  failed" differ). Windows is refused; WSL is Linux. No image, bytes that
  are not a PNG, over 20 MB, or no tool: exit 1 before any api call,
  naming the tool to install.
- One ssh command over the project's multiplexed connection (as `cp`)
  writes stdin to `/tmp/repose-paste/<UTC yyyymmdd-hhmmss-ms>.png` (umask
  077: directory 0700, file 0600, owned by `dev`), refusing a directory
  that is a symlink or not `dev`'s, and first deletes pastes over a day
  old and all but the newest 50.
- In the same command, the path goes into the target pane with `tmux
  set-buffer` and `paste-buffer -p`: a bracketed paste when the program
  asked for one, which is how a terminal delivers a dropped file and what
  Claude Code attaches. No Enter. The target is the session's current
  window's active pane, or `=<slug>:<NAME>` with `--window`. A window
  that does not exist leaves the file saved and exits 1 with its path.
- `--print` saves and prints the guest path only.
- Nothing is logged; the path and the image stay between the laptop and
  the guest.

A terminal key binding that runs it (kitty, WezTerm) is documented on
/docs/run-and-attach, not shipped.

## Depends on

Workstreams 07 (cli), 05 (projects, certs, ops), 04 (guestd SetupProject,
tmux control, send-keys idle wait), 02 (guest base, wrappers), 06 (gateway),
12 (build streaming).

## Deferred

Worktrees by default (DECISIONS R4-10 chose warn-and-proceed; I-253 made
`--worktree` opt-in). A command that lists or removes worktrees. Queueing prompts for
when the current agent finishes. Web terminal in the dashboard (R4-18).
