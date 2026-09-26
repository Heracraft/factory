---
title: CLI reference
description: Every repose command and flag, config.toml, environment variables and the files the CLI keeps.
section: Reference
order: 40
---

`repose --help`, `repose help COMMAND` and `repose COMMAND --help` print the same in your terminal.

## Which project

Commands that act on a project use, in order: the `PROJECT` argument, `--project NAME`, the `REPOSE_PROJECT` environment variable, then the current checkout's git remote.

Global flags: `--project NAME`, `-v`/`--verbose` (debug output to stderr), `--version`, and `--api-url URL` (see [Other servers](#other-servers)).

## Working on a project

### `repose run [PROMPT]`

Create or start this checkout's machine, sync, and attach. With a prompt, start an agent and type the prompt into it. See [Run and attach](/docs/run-and-attach).

| Flag                      |                                                                   |
| ------------------------- | ----------------------------------------------------------------- |
| `--agent NAME`            | `claude`, `codex`, `opencode`, `gemini` or `pi`.                  |
| `--no-attach`             | Don't attach afterwards.                                          |
| `--worktree`              | Start the agent in its own git worktree. Needs a prompt.          |
| `--no-sync`               | Skip the git sync and the copied logins.                          |
| `--stash-remote`          | Stash the machine's uncommitted changes before syncing.           |
| `--discard-remote`        | Discard the machine's uncommitted changes before syncing.         |
| `--size small\|large\|xl` | Size of a new project.                                            |
| `--name NAME`             | Project name, for a directory with no remote or a second project. |

### `repose attach [PROJECT]`

Attach to the project's tmux session without syncing.

`run` and `attach` print one line when another of your projects is running idle, once per idle stretch. An `attach` that reuses an open connection makes no api call and skips it.

### `repose open [PORT]`

Forward one port to your laptop and open it in the browser, until `Ctrl-C`.

| Flag               |                                                              |
| ------------------ | ------------------------------------------------------------ |
| `--local-port N`   | Port on the laptop. Default: the same, or the next free one. |
| `--no-browser`     | Print the URL only.                                          |
| `--desktop`        | Start the machine's desktop and forward it.                  |
| `--desktop --stop` | Stop the desktop.                                            |

### `repose cp [-r] SRC DST`

Copy files with `scp`. One side is `PROJECT:PATH`, or `:PATH` for this checkout's project. Relative machine paths start at the checkout. `-r`/`--recursive` copies directories.

### `repose paste [PROJECT]`

Copy the image on your clipboard to `/tmp/repose-paste/` on the machine and paste its path into the tmux session's current pane, where Claude Code attaches it. Nothing is sent with it; you press Enter. See [Paste an image](/docs/run-and-attach#paste-an-image).

| Flag            |                                                                  |
| --------------- | ---------------------------------------------------------------- |
| `--window NAME` | Paste into this tmux window (name or number) instead.            |
| `--print`       | Copy the image and print its path on the machine; paste nothing. |

### `repose scan [DIR]`

List the tools the next `repose run` would install on the machine, and why. Installs nothing. `--json` for JSON.

## Projects

### `repose projects`

Every project in a table, with a line under it for each running project nobody has used for a day ([Idle machines](/docs/lifecycle#idle-machines)). `--json` for full records, `--destroyed` for destroyed projects that can still be restored (with `--all`, every one).

### `repose status [PROJECT]`

One project in detail, including processes listening on ports, and the idle line when it has had nobody on it for a day. `--json`, `--watch` (every 5 seconds).

### `repose start [PROJECT]`

Start a stopped machine, or restart one in `error`. Doesn't sync.

### `repose stop [PROJECT]`

Stop the machine and snapshot its disk. `--no-snapshot` skips the snapshot.

### `repose destroy [PROJECT]`

Delete the machine and disk; a final snapshot is kept 30 days. `-y`/`--yes` skips the question (required without a terminal). `--wait` waits until it's done.

### `repose restore [NAME]`

Bring back a project destroyed in the last 30 days. `--as NEW-NAME` for another name, `--snapshot ID` for an older snapshot.

### `repose fork [PROJECT]`

Snapshot the project now and start copies of it as new projects, each on its own machine, so several agents can try different approaches from the same starting point. `-n`/`--count N` makes N copies (1 to 10, default 1), named `PROJECT-fork-1`, `PROJECT-fork-2` and so on; `--name NAME` names them `NAME-1`, `NAME-2`. `--size` sets their size (default: the project's). `--snapshot ID` copies one of the project's snapshots instead of taking a new one. `--prompt TEXT` starts the agent in every copy with that prompt (`--agent` picks the agent). `--json` prints the copies as JSON. The project itself keeps running. Each copy counts toward your project limit and is billed like any project. If the copies would take you past the limit, nothing is created. See [Fork a project](/docs/lifecycle#fork-a-project).

### `repose resize SIZE`

Grow the project's disk, for example `repose resize 80G`. Disks can't shrink, and the larger disk is billed from then on.

### `repose logs [PROJECT]`

`--kind console|build|ops` (default `console`), `--since 1h`, `-f`/`--follow` to follow, `--json`.

### `repose events [PROJECT]`

`--since 72h` (default `24h`), `-f`/`--follow` to follow, `--json`.

### `repose questions [PROJECT]`

The questions agents are waiting on you to answer, from all projects or one. Each shows the project, the agent, how long ago it asked, when it expires, the question and how to answer it. `--json` for JSON. See [Notifications](/docs/notifications#agents-can-message-you-and-ask-questions).

### `repose reply [PROJECT] [ANSWER...]`

Answer a waiting question: `repose reply todo-app yes`. The first word is the project when it names one with a waiting question; otherwise every word is the answer, which works when only one question is waiting. With several waiting, it lists them and sends nothing; name the project or pass `--question ID` (the id `repose questions` shows). With no answer, it asks for one in the terminal. When the question has options, the answer must be one of them. `--json` prints the answered question.

## Snapshots

| Command                                |                                                                                                                    |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `repose snapshots list`                | `--json` for JSON.                                                                                                 |
| `repose snapshots create`              | Take one now.                                                                                                      |
| `repose snapshots restore SNAPSHOT_ID` | Replace a stopped project's disk. `--as-new NAME` restores into a new project instead; `--yes` skips the question. |

## Secrets

| Command                   |                                                                 |
| ------------------------- | --------------------------------------------------------------- |
| `repose secrets set NAME` | Asks for the value. `--from-file PATH` or `--from-env` instead. |
| `repose secrets list`     | Names and dates, never values.                                  |
| `repose secrets rm NAME`  | Delete it.                                                      |

## Configuration

| Command                           |                                                        |
| --------------------------------- | ------------------------------------------------------ |
| `repose config add PACKAGE...`    | Add menu entries or nixpkgs packages, build and apply. |
| `repose config remove PACKAGE...` | Remove them again. Alias `rm`.                         |
| `repose config show`              | Print the Nix file. `--revisions` lists revisions.     |
| `repose config edit`              | Edit in `$EDITOR`, apply on save.                      |
| `repose config apply [PATH]`      | Apply a file. Default `./repose.nix`.                  |

## Account

| Command                             |                                                                                                                  |
| ----------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `repose login`                      | Log in with a code in any browser. `--no-browser` is the same; `--browser`, see [Other servers](#other-servers). |
| `repose logout`                     | Log out and revoke SSH certificates. `--purge` removes the CLI's files.                                          |
| `repose notify set`                 | `--email on\|off`, `--ntfy URL\|none`.                                                                           |
| `repose notify test`                | Send a test on every channel that's on.                                                                          |
| `repose version`                    | Print the version.                                                                                               |
| `repose completion bash\|zsh\|fish` | Print a shell completion script.                                                                                 |
| `repose help [COMMAND]`             | Print help for a command.                                                                                        |
| `repose mcp forward`                | Reserved, not available yet. Prints what works today.                                                            |
| `repose browser bridge`             | Reserved, not available yet. Prints what works today.                                                            |

## config.toml

`~/.config/repose/config.toml` is optional.

```toml
default_class = "small"
default_agent = "codex"

[sync]
exclude = ["dist", "*.mp4"]
```

| Key               | Default  |                                                                      |
| ----------------- | -------- | -------------------------------------------------------------------- |
| `default_class`   | `large`  | Size of new projects.                                                |
| `default_agent`   | `claude` | Agent for new projects.                                              |
| `sync.exclude`    | none     | More gitignore-style patterns the sync leaves out.                   |
| `api_url`         | hosted   | See [Other servers](#other-servers).                                 |
| `logto_issuer`    | hosted   | The login server. See [Other servers](#other-servers).               |
| `logto_client_id` | hosted   | The CLI's application id there. See [Other servers](#other-servers). |

## Environment variables

| Variable               |                                                                                                                             |
| ---------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| `REPOSE_PROJECT`       | The project to act on, like `--project`.                                                                                    |
| `REPOSE_NO_FORWARD=1`  | Don't forward ports automatically while attached.                                                                           |
| `REPOSE_TIMING=1`      | Print how long each step of `run` and `attach` took.                                                                        |
| `REPOSE_NO_SPINNER=1`  | One line per step instead of a progress line. `TERM=dumb` does the same.                                                    |
| `REPOSE_NO_FASTPATH=1` | Check with the server before every connection instead of reusing the last one. Slower; for when a connection keeps failing. |
| `REPOSE_NO_BROWSER=1`  | Never open a browser, even with `repose login --browser`.                                                                   |
| `REPOSE_API_URL`       | Like `--api-url`.                                                                                                           |
| `REPOSE=1`             | Set on every repose machine, so scripts can tell where they run.                                                            |
| `XDG_CONFIG_HOME`      | If set, the CLI's files are in `$XDG_CONFIG_HOME/repose/`.                                                                  |
| `CLAUDE_CONFIG_DIR`    | Where your laptop's Claude Code setup is copied from, instead of `~/.claude`.                                               |
| `VISUAL`, `EDITOR`     | The editor for `repose config edit`. Default `vi`.                                                                          |
| `WAYLAND_DISPLAY`      | On Linux, `repose paste` reads the Wayland clipboard with `wl-paste` when this is set.                                      |
| `DISPLAY`              | Otherwise it reads the X11 clipboard with `xclip`.                                                                          |

## Other servers

For a test or self-hosted repose server rather than the hosted one: `--api-url URL`, `REPOSE_API_URL` or `api_url` in config.toml pick the API, and `logto_issuer` and `logto_client_id` the login server and the CLI's application there. `repose login --browser` logs in through a browser on this computer instead of a code, for a login application that allows local redirects; the hosted one doesn't.

## Files on your laptop

| Path                |                                                                                                                   |
| ------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `~/.config/repose/` | Your login (mode 0600; on macOS the token is in the keychain), `config.toml`, and caches that are safe to delete. |
| `~/.ssh/repose/`    | The CLI's own SSH key and 12-hour certificate, and one `Host` block per project.                                  |
| `~/.ssh/config`     | One added line: `Include ~/.ssh/repose/config`.                                                                   |

`repose logout --purge` removes all of these.

## Exit codes

| Code | Meaning                                                |
| ---- | ------------------------------------------------------ |
| 0    | Worked.                                                |
| 1    | Failed; the message says why.                          |
| 2    | Wrong usage.                                           |
| 3    | Not logged in.                                         |
| 4    | No such project.                                       |
| 5    | The machine isn't running.                             |
| 6    | The machine has uncommitted changes; the sync stopped. |
| 7    | Account or payment problem.                            |
| 8    | No capacity right now; try again in a few minutes.     |
| 10   | The configuration build failed.                        |
| 130  | Interrupted with `Ctrl-C`.                             |

Once `run` or `attach` has connected you, the exit code is `ssh`'s. `repose cp` returns `scp`'s. `repose paste` exits 1 when there is no image on the clipboard or no tool to read it, and says which tool to install.
