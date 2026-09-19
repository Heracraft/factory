# Run and attach

`repose run` is the whole product in one command. With no arguments it puts
you inside your project's tmux session. With a prompt it starts an agent in a
new tmux window, types the prompt, and leaves it running whether or not you
stay attached.

## What the user sees

First run on a new project:

```
$ repose run
Creating project todo-app (github.com/heracraft/todo-app) as large on host az-eastus-01
Building environment (base 2026.09.15 + your config) ... 41s
Starting guest ... 4s
Syncing: 3 changed files, 1 untracked, 12 KB
dev@todo-app:~/todo-app$
```

Run with a prompt:

```
$ repose run "finish the auth flow, run the tests, commit when green"
Starting claude in window todo-app:claude
Attached. Detach with C-b d; the agent keeps running.
```

Run with another agent and an explicit size for a new project:

```
$ repose run --agent codex --size xl "port the build to bun"
```

Attach to an existing session later, from any machine where you are logged in:

```
$ repose attach
```

Run a prompt while an agent is already running:

```
$ repose run "also update the README"
warning: claude is already running in todo-app:claude on the same working tree.
Starting a second claude in window todo-app:claude-2. Two agents on one tree
can conflict; use `git worktree` inside the guest if that matters.
```

Trying to run a stopped project:

```
$ repose run
todo-app is stopped. Starting ... 4s
```

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
- The prompt is typed into the TUI only once the TUI is up. guestd waits
  for the pane to be idle for 1 second before `send-keys`, then sends the
  prompt and Enter. A prompt containing newlines is sent as one paste
  (bracketed paste), not as separate Enter presses.
- After starting the agent the CLI attaches to that window unless
  `--detach` was given. Detaching (`C-b d`) never stops anything.
- `repose attach` attaches to the current window. `repose attach --window
  claude` attaches to a named window. If the session does not exist (guest
  rebooted and guestd failed to recreate it), the CLI recreates it via
  guestd and says so.
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
   guest. A guest in state `building` shows the build log; `error` shows
   the last error and exits 5.
3. Get or refresh the SSH certificate; write the SSH config block.
4. Sync (sync-at-launch.md). Skipped with `--no-sync`.
5. Sync credential files (secrets.md).
6. Start the agent window if a prompt was given, then attach.

Running `repose run` twice in a row attaches twice and changes nothing else.
A test asserts that the second run makes no `POST` to the API except the
certificate refresh, if due.

Timing that must hold on a healthy host:

- A stopped guest starts and is attachable in under 10 seconds.
- A new project with an empty fragment is attachable in under 60 seconds,
  because the base closure is already in the host store.

Failure output:

- Not logged in: exit 3, `run \`repose login\` first`.
- No card: exit 7 with the dashboard billing URL.
- Host capacity exhausted: exit 8, `no host has room for a large guest right
  now; try again in a few minutes or pick --size small`. The API also raises
  a capacity alert.
- Build failed: exit 10, the Nix error verbatim, the fragment line if known,
  and `edit with \`repose config edit\``.
- Gateway unreachable or certificate rejected: the ssh error verbatim plus
  `run \`repose login\` again if this persists`.

## Depends on

Workstreams 07 (cli), 05 (projects, certs, ops), 04 (guestd SetupProject,
tmux control, send-keys idle wait), 02 (guest base, wrappers), 06 (gateway),
12 (build streaming).

## Deferred

`repose run --worktree` to start a second agent in a git worktree
automatically (DECISIONS R4-10 chose warn-and-proceed). Queueing prompts for
when the current agent finishes. Web terminal in the dashboard (R4-18).
