---
title: Run and attach
description: Start agents, move around the tmux session, get back to it later, and connect with SSH or an editor.
section: Using repose
order: 10
---

## Start an agent with a prompt

```
repose run "migrate the date handling to Temporal and fix the tests that break"
```

Quotes are optional; everything after the flags is the prompt. A one-word prompt that is the name of one of your projects is refused as a likely slip (exit code 2); to send it anyway, name the agent: `repose run --agent claude todo-app`. `run` creates or starts the machine, [syncs](/docs/sync) your checkout, opens a new tmux window, starts the agent there, types your prompt and attaches you to it.

Without a prompt, `repose run` syncs and drops you in the last active window.

Pick a different agent for one prompt with `--agent`:

```
repose run --agent codex "port the build scripts to bun"
```

The agent is the normal interactive program, the same as running `claude` yourself, so its history and any questions it asks are on screen when you attach.

If that agent already has a window, the new one is named `claude-2`, then `claude-3`, and so on, and the CLI warns that the agents share one working tree.

Running `repose run` twice in a row is safe. The second one finds nothing new to copy.

## Several agents, separate trees

`--worktree` starts the agent in its own git worktree, so it doesn't edit the files another agent is working on:

```
$ repose run --worktree "try the other approach"
Worktree: ~/todo-app-claude-2 on branch repose/claude-2
```

The worktree is a folder next to your checkout on the machine, on a new branch from the checkout's last commit. Uncommitted changes in the checkout aren't in it. `repose run` never syncs it, and what the agent does there doesn't count as changes on the machine. Commit on the branch and merge or push it like any other. On your laptop, `git fetch repose` brings it as `repose/repose/claude-2` ([Getting work back](/docs/sync#getting-work-back)).

Each `--worktree` run makes a new one. They stay until you remove them, from the checkout on the machine:

```
git worktree remove ~/todo-app-claude-2
git branch -D repose/claude-2
```

The machine's checkout needs at least one commit; otherwise `--worktree` is refused with exit code 2. Dependencies aren't shared, so the agent installs them again in the worktree, and gitignored files such as `.env` aren't copied.

## Detach and come back

Press `Ctrl-b`, let go, then `d`. You're back on your laptop and everything on the machine keeps running. Closing the terminal, losing Wi-Fi or closing the laptop does the same.

To get back, from the checkout or from anywhere:

```
repose attach
repose attach todo-app
```

`attach` doesn't sync your checkout, so it's safe to use from a second computer whose copy is older. It doesn't start a stopped machine either; it tells you to run `repose start`.

Several terminals can be attached at once, from one computer or several. They see the same windows.

## tmux keys

Each machine has one tmux session. Its first window, `shell`, opens in your checkout; agents get windows next to it. Press `Ctrl-b`, then:

| Key     | Action                                          |
| ------- | ----------------------------------------------- |
| `d`     | Detach.                                         |
| `w`     | Pick a window from a list.                      |
| `n` `p` | Next or previous window.                        |
| `c`     | New window with a shell.                        |
| `[`     | Scroll back. Arrow keys or Page Up; `q` leaves. |

The mouse works too: click a window name to switch, scroll to go back through output.

Shift+Enter starts a new line in Claude Code instead of sending the prompt, when your terminal reports modified keys to tmux (xterm's modifyOtherKeys; Ghostty, WezTerm, iTerm2 and xterm do, Apple's Terminal doesn't). If Shift+Enter still sends the prompt, type `\` and then Enter, or press Ctrl+J. Links an agent prints are clickable in terminals that support links (OSC 8), and a program in the window you're looking at can send escape sequences through tmux to your terminal.

## See what's running, run one command

`repose ps` lists the tmux windows without attaching: what runs in each and when it last printed something. `*` is the window `attach` opens on.

```
$ repose ps
WINDOW     COMMAND  ACTIVE
0:shell    bash     3h ago
1:claude*  claude   now
2:codex    codex    12m ago
```

An agent that's working usually shows `now`; one that has been waiting for you shows roughly how long. COMMAND is the program's name only, never its arguments.

`repose exec` runs one command in the checkout on the machine and gives you its output and exit code, the way `docker exec` does. The command gets what an agent there gets: your [secrets](/docs/secrets) as environment variables and the project's dev shell (its `.envrc`, or its `flake.nix` dev shell). Loading it prints nothing unless it takes more than 2 seconds or fails.

```
$ repose exec -- npm test
$ repose exec todo-app -- git status --short
$ repose exec -it -- psql
```

Everything after `--` is the command. Without `-i` it reads no input, and without `-t` it has no terminal; `-it` is for something interactive, like a REPL. In a script, `repose exec -- make check && echo passed` works as you'd expect, since the exit code is the command's.

`repose ssh` opens a plain shell in the checkout instead of the tmux session, and `exit` closes it. Start long jobs in tmux (`repose attach`), where they outlive the connection.

Like `attach`, these start no machine: a stopped one gets you exit code 5 and the command to start it.

## Drop a file or paste an image

While you're attached, drag a file onto the terminal, or press `Ctrl+V` with a screenshot on your laptop's clipboard. The file is copied to `/tmp/repose-paste/` on the machine and its path there is pasted where your cursor is:

```
❯ [Image #1] the button overlaps the footer on this screen
```

Claude Code shows an image as `[Image #1]`; add your words and press Enter. Other agents, and the shell, get the path as text, and can open the file.

- A file from your checkout isn't copied. You get its path in the machine's checkout, such as `/home/dev/todo-app/docs/mockup.png`. If the machine's copy isn't there yet or differs in size, the file is copied like any other.
- Drop several files at once to paste several paths.
- Up to 20 files and 20 MB per file. A bigger drop pastes your laptop's path unchanged, and the tmux status line says why; use [`repose cp`](/docs/sync#single-files) for large files.
- Only you and the machine's `dev` user can read the copies. Copies older than a day, and all but the newest 50, are deleted at the next copy.
- A paste that is nothing but paths of files on your laptop counts as a drop, so pasting a copied path works too. Paths under system folders such as `/etc`, `/usr` and `/nix` are pasted as they are, and so are hidden files and anything in a hidden folder such as `~/.ssh`: those are never copied.

Any terminal that types a dropped file's path works: plain, quoted, with backslashes before spaces, or as a `file://` address.

On macOS, press Ctrl+V, not Cmd+V: with only an image on the clipboard, Cmd+V sends the terminal nothing. Ctrl+V reads the clipboard with `pngpaste` if you have it, otherwise `osascript`. On Linux it uses `wl-paste` (from wl-clipboard) under Wayland and `xclip` under X11. With no image on the clipboard, Ctrl+V is an ordinary Ctrl+V, so vim and the shell behave as usual. With one, Ctrl+V pastes the image in every window.

`REPOSE_INPUT_PROXY=0` turns this off: `run` and `attach` then hand your terminal straight to `ssh`, and a drop pastes your laptop's path. On Windows it's always off; copy the file with `repose cp FILE :/tmp/` and type its path.

### From a script or another window

`repose paste` copies the image on the clipboard and pastes its path into the tmux window you were last in, without a key press:

```
repose paste
```

- `repose paste todo-app` from anywhere; `--window claude-2` for another window; `--print` to only print the path.
- It reads the clipboard with the same tools as Ctrl+V. Under WSL it reads the Linux clipboard, which may not have images copied in Windows.

## Useful flags

```
repose run --no-attach "..."     # start the agent and return to your shell
repose run --no-sync             # skip the sync
repose run --size xl             # size of a new project
repose run --name scratch        # a directory with no git remote
repose run --project todo-app    # a project other than this checkout's
```

The full list is in the [CLI reference](/docs/cli#repose-run-prompt).

## SSH and editors

Every project is also an SSH host called `<project>.repose`, so `ssh`, `scp`, `rsync`, git and editors reach it with no `repose` command first. `repose code` opens the checkout in VS Code, Cursor or Zed:

```
ssh todo-app.repose
repose code todo-app
```

Plain `ssh` doesn't attach to tmux; run `tmux attach` for that. [SSH and editors](/docs/ssh-and-editors) has the rest: scp and rsync, git over SSH, each editor by hand, and what to do when a connection fails.

mosh doesn't work: it needs a UDP connection straight to the machine, and the only way in is SSH through repose. A dropped connection loses nothing, since the agents keep running in tmux; `repose attach` gets you back.

## Time zone

`run` and `attach` set the machine's time zone to your laptop's. New shells pick it up. Until the first `run`, a machine uses the time zone on the dashboard's **Settings** page.

## When the machine stops

A stop ends every process, agents included. After the next start, the tmux session has a fresh `shell` window and the agents' windows are gone. To continue a Claude Code conversation, run `claude --resume` in the checkout and pick it.

## Timing

`REPOSE_TIMING=1 repose run` prints how long each step took. A `run` into a running machine with nothing new to sync usually takes about a second; starting a stopped one, about 10.
