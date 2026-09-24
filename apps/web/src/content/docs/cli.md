---
title: CLI reference
description: Every repose command and flag.
section: Reference
order: 41
---

`repose --help` and `repose <command> --help` print the same information in your terminal.

## Picking the project

Commands that act on one project find it in this order:

1. The `PROJECT` argument, for commands that take one.
2. `--project NAME` (a project's name or id).
3. The `REPOSE_PROJECT` environment variable.
4. The current checkout's git remote.

Naming two different projects at once (an argument and `--project` that disagree) is an error.

## Global flags

| Flag              |                                                                 |
| ----------------- | --------------------------------------------------------------- |
| `--project NAME`  | The project to act on. Same as `REPOSE_PROJECT`.                |
| `-v`, `--verbose` | Debug logging to stderr.                                        |
| `--api-url URL`   | A different API. Same as `REPOSE_API_URL`. You won't need this. |
| `--version`       | Print the version.                                              |

## Account

### `repose login`

Log in with GitHub. Prints a URL and a code to enter in any browser. See [Install and log in](/docs/install#log-in).

### `repose logout`

Revoke your SSH certificates and forget your login.

| Flag      |                                                                                              |
| --------- | -------------------------------------------------------------------------------------------- |
| `--purge` | Also delete `~/.ssh/repose/`, `~/.config/repose/` and the `Include` line in `~/.ssh/config`. |

### `repose notify set`

| Flag               |                                                    |
| ------------------ | -------------------------------------------------- |
| `--email on\|off`  | Email notifications.                               |
| `--ntfy URL\|none` | Send push notifications to this ntfy URL, or stop. |

Prints the resulting settings. See [Notifications](/docs/notifications).

### `repose notify test`

Send a test notification on every channel that's on, and print `ok` or `error` for each. Exits 1 if none worked.

### `repose version`

Print the CLI's version.

## Working on a project

### `repose run [PROMPT]`

Create or start this checkout's machine, sync your work to it and attach. With a prompt, start an agent and type the prompt into it first. See [Run and attach](/docs/run-and-attach).

| Flag                      |                                                                                                                                                                                    |
| ------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--agent NAME`            | `claude`, `codex`, `opencode`, `gemini` or `pi`. Default: the project's, which is `claude` unless [`default_agent`](/docs/cli-config) said otherwise when the project was created. |
| `--no-attach`             | Don't attach afterwards.                                                                                                                                                           |
| `--no-sync`               | Skip the git sync and the tool logins.                                                                                                                                             |
| `--stash-remote`          | Stash the machine's uncommitted changes before syncing.                                                                                                                            |
| `--discard-remote`        | Discard the machine's uncommitted changes before syncing.                                                                                                                          |
| `--size small\|large\|xl` | Size for a new project.                                                                                                                                                            |
| `--name NAME`             | Project name, for a directory with no remote, or a second project for the same repository.                                                                                         |

### `repose attach [PROJECT]`

Attach to the project's tmux session without syncing.

### `repose open [PORT]`

Forward one port from the machine to your laptop and open it in the browser. Runs until `Ctrl-C`. See [Ports and localhost](/docs/ports#repose-open).

| Flag               |                                                                                 |
| ------------------ | ------------------------------------------------------------------------------- |
| `--local-port N`   | Use this port on the laptop. Default: the same as `PORT`, or the next free one. |
| `--no-browser`     | Print the URL without opening a browser.                                        |
| `--desktop`        | Start the machine's desktop and forward it. See [Browser](/docs/browser).       |
| `--desktop --stop` | Stop the desktop. (Not in a release yet.)                                       |

### `repose cp [-r] SRC DST`

Copy files between the laptop and a machine with `scp`. One side is `PROJECT:PATH`, or `:PATH` for this checkout's project. Relative paths on the machine start at the checkout. `-r` copies directories. Not in a release yet.

### `repose scan [DIR]`

List the tools the next `repose run` would install on the machine and why, without installing anything or contacting the machine. `--json` for JSON. Not in a release yet.

## Projects

### `repose projects`

Table of every project.

| Flag          |                                                                                                                       |
| ------------- | --------------------------------------------------------------------------------------------------------------------- |
| `--json`      | Print the full records.                                                                                               |
| `--destroyed` | List destroyed projects that can still be restored, and until when.                                                   |
| `--all`       | With `--destroyed`: every destroyed project, including older ones with the same name, with the id that restores each. |

### `repose status [PROJECT]`

One project in detail. See [Status, logs and the dashboard](/docs/status).

| Flag      |                           |
| --------- | ------------------------- |
| `--json`  | Print the project record. |
| `--watch` | Refresh every 5 seconds.  |

### `repose start [PROJECT]`

Start a stopped machine, or restart one in `error`. Doesn't sync.

### `repose stop [PROJECT]`

Shut the machine down and snapshot its disk.

| Flag            |                    |
| --------------- | ------------------ |
| `--no-snapshot` | Skip the snapshot. |

### `repose destroy [PROJECT]`

Delete the machine and its disk. A final snapshot is kept 30 days.

| Flag          |                                                             |
| ------------- | ----------------------------------------------------------- |
| `-y`, `--yes` | Don't ask for confirmation. Required without a terminal.    |
| `--wait`      | Wait until the destroy is finished and report how it ended. |

### `repose restore [NAME]`

Bring back a project destroyed in the last 30 days, from its newest snapshot. Inside the checkout, `NAME` can be left out.

| Flag            |                                                                                               |
| --------------- | --------------------------------------------------------------------------------------------- |
| `--as NEW-NAME` | Restore under another name, when the old one is taken.                                        |
| `--snapshot ID` | Restore this snapshot instead of the newest. `repose snapshots list --project ID` lists them. |

### `repose logs [PROJECT]`

| Flag                         |                               |
| ---------------------------- | ----------------------------- |
| `--kind console\|build\|ops` | Which log. Default `console`. |
| `--since DURATION`           | For example `1h`.             |
| `-f`, `--follow`             | Keep printing new lines.      |
| `--json`                     | One JSON object per line.     |

### `repose events [PROJECT]`

| Flag               |                                       |
| ------------------ | ------------------------------------- |
| `--since DURATION` | Default `24h`.                        |
| `-f`, `--follow`   | Poll for new events every 10 seconds. |
| `--json`           | One JSON object per event.            |

## Snapshots

### `repose snapshots list`

`--json` for JSON.

### `repose snapshots create`

Take a snapshot now.

### `repose snapshots restore SNAPSHOT_ID`

Replace the project's disk with the snapshot. The machine has to be stopped.

| Flag            |                                                                |
| --------------- | -------------------------------------------------------------- |
| `--as-new NAME` | Restore into a new project instead. The original is untouched. |
| `--yes`         | Don't ask for confirmation.                                    |

## Secrets

### `repose secrets set NAME`

Store a secret for the project. Asks for the value without showing it.

| Flag               |                                                      |
| ------------------ | ---------------------------------------------------- |
| `--from-file PATH` | Read the value from a file.                          |
| `--from-env`       | Read the value from the environment variable `NAME`. |

### `repose secrets list`

Names and when each was last set. Never values.

### `repose secrets rm NAME`

Delete a secret.

## Configuration

### `repose config show`

Print the project's Nix fragment. `--revisions` lists revisions with their status instead.

### `repose config edit`

Open the fragment in `$EDITOR`, then build and apply it when you save and quit.

### `repose config apply [PATH]`

Build and apply a fragment from a file. Default `./repose.nix`.

### `repose config add PACKAGE...`

Add menu entries or nixpkgs packages, then build and apply. Not in a release yet.

### `repose config remove PACKAGE...`

Remove packages added with `config add`. Alias `rm`. Not in a release yet.

## Other

### `repose completion SHELL`

Print a completion script for `bash`, `zsh`, `fish` or `powershell`. For zsh:

```
repose completion zsh > "${fpath[1]}/_repose"
```

Project names complete for commands that take one.

### `repose mcp forward`, `repose browser bridge`

Reserved for features that aren't built yet. They print a note and exit 0.
