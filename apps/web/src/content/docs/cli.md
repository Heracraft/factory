---
title: CLI reference
description: Every repose command and flag, config.toml, environment variables and the files the CLI keeps.
section: Reference
order: 40
---

`repose --help` and `repose <command> --help` print the same in your terminal.

## Which project

Commands that act on a project use, in order: the `PROJECT` argument, `--project NAME`, the `REPOSE_PROJECT` environment variable, then the current checkout's git remote.

Global flags: `--project NAME`, `-v` (debug output to stderr), `--version`.

## Working on a project

### `repose run [PROMPT]`

Create or start this checkout's machine, sync, and attach. With a prompt, start an agent and type the prompt into it. See [Run and attach](/docs/run-and-attach).

| Flag                      |                                                                   |
| ------------------------- | ----------------------------------------------------------------- |
| `--agent NAME`            | `claude`, `codex`, `opencode`, `gemini` or `pi`.                  |
| `--no-attach`             | Don't attach afterwards.                                          |
| `--no-sync`               | Skip the git sync and the copied logins.                          |
| `--stash-remote`          | Stash the machine's uncommitted changes before syncing.           |
| `--discard-remote`        | Discard the machine's uncommitted changes before syncing.         |
| `--size small\|large\|xl` | Size of a new project.                                            |
| `--name NAME`             | Project name, for a directory with no remote or a second project. |

### `repose attach [PROJECT]`

Attach to the project's tmux session without syncing.

### `repose open [PORT]`

Forward one port to your laptop and open it in the browser, until `Ctrl-C`.

| Flag               |                                                              |
| ------------------ | ------------------------------------------------------------ |
| `--local-port N`   | Port on the laptop. Default: the same, or the next free one. |
| `--no-browser`     | Print the URL only.                                          |
| `--desktop`        | Start the machine's desktop and forward it.                  |
| `--desktop --stop` | Stop the desktop.                                            |

### `repose cp [-r] SRC DST`

Copy files with `scp`. One side is `PROJECT:PATH`, or `:PATH` for this checkout's project. Relative machine paths start at the checkout.

### `repose scan [DIR]`

List the tools the next `repose run` would install on the machine, and why. Installs nothing. `--json` for JSON.

## Projects

### `repose projects`

Every project in a table. `--json` for full records, `--destroyed` for destroyed projects that can still be restored (with `--all`, every one).

### `repose status [PROJECT]`

One project in detail, including processes listening on ports. `--json`, `--watch` (every 5 seconds).

### `repose start [PROJECT]`

Start a stopped machine, or restart one in `error`. Doesn't sync.

### `repose stop [PROJECT]`

Stop the machine and snapshot its disk. `--no-snapshot` skips the snapshot.

### `repose destroy [PROJECT]`

Delete the machine and disk; a final snapshot is kept 30 days. `-y`/`--yes` skips the question (required without a terminal). `--wait` waits until it's done.

### `repose restore [NAME]`

Bring back a project destroyed in the last 30 days. `--as NEW-NAME` for another name, `--snapshot ID` for an older snapshot.

### `repose logs [PROJECT]`

`--kind console|build|ops` (default `console`), `--since 1h`, `-f` to follow, `--json`.

### `repose events [PROJECT]`

`--since 72h` (default `24h`), `-f` to follow, `--json`.

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

| Command                             |                                                                         |
| ----------------------------------- | ----------------------------------------------------------------------- |
| `repose login`                      | Log in with a code in any browser.                                      |
| `repose logout`                     | Log out and revoke SSH certificates. `--purge` removes the CLI's files. |
| `repose notify set`                 | `--email on\|off`, `--ntfy URL\|none`.                                  |
| `repose notify test`                | Send a test on every channel that's on.                                 |
| `repose version`                    | Print the version.                                                      |
| `repose completion bash\|zsh\|fish` | Print a shell completion script.                                        |

`repose mcp forward` and `repose browser bridge` are reserved and not available yet.

## config.toml

`~/.config/repose/config.toml` is optional.

```toml
default_class = "small"     # size of new projects; default large
default_agent = "codex"     # agent for new projects; default claude

[sync]
exclude = ["dist", "*.mp4"] # more gitignore-style patterns to leave out
```

## Environment variables

| Variable              |                                                      |
| --------------------- | ---------------------------------------------------- |
| `REPOSE_PROJECT`      | The project to act on, like `--project`.             |
| `REPOSE_NO_FORWARD=1` | Don't forward ports automatically while attached.    |
| `REPOSE_TIMING=1`     | Print how long each step of `run` and `attach` took. |
| `REPOSE_NO_SPINNER=1` | One line per step instead of a progress line.        |

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
