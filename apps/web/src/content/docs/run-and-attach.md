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

The worktree is a folder next to your checkout on the machine, on a new branch from the checkout's last commit. Uncommitted changes in the checkout aren't in it. `repose run` never syncs it, and what the agent does there doesn't count as changes on the machine. Commit on the branch and merge or push it like any other.

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

## Paste an image

`Ctrl-V` in Claude Code reads the clipboard of the computer it runs on, which is the machine, not your laptop. To give an agent a screenshot, copy it on your laptop and run, in the checkout:

```
repose paste
```

The image goes to `/tmp/repose-paste/` on the machine, and its path is pasted into the tmux window you were last in, as if you had dropped the file there. Claude Code shows it as an attached image; add your words and press Enter. Other agents get the path as text.

- `repose paste todo-app` from anywhere; `--window claude-2` for another window; `--print` to only print the path.
- On macOS it uses `pngpaste` if you have it, otherwise `osascript`. On Linux it uses `wl-paste` (from wl-clipboard) under Wayland and `xclip` under X11.
- Windows isn't supported. Under WSL it reads the Linux clipboard, which may not have images copied in Windows.
- Only you and the machine's `dev` user can read the files. Up to 20 MB per image; pastes older than a day, and all but the newest 50, are deleted at the next paste.

To paste with a key, bind one in your terminal to run `repose paste` on the laptop. Use the full path from `command -v repose` if the terminal doesn't find it.

kitty, in `kitty.conf` (runs in the directory of the window you're in, so the checkout's project is found):

```
map ctrl+alt+v launch --type=background --cwd=current repose paste
```

WezTerm, in `wezterm.lua`, for one project:

```lua
config.keys = {
  {
    key = 'v',
    mods = 'CTRL|ALT',
    action = wezterm.action_callback(function()
      wezterm.background_child_process { 'repose', 'paste', 'todo-app' }
    end),
  },
}
```

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

Every project is also an SSH host called `<project>.repose`, so anything that speaks SSH works:

```
ssh todo-app.repose
scp todo-app.repose:~/todo-app/report.html .
ssh -L 9229:localhost:9229 todo-app.repose
```

You log in as `dev`. Plain `ssh` doesn't attach to tmux; run `tmux attach` for that.

**VS Code and Cursor:** with the Remote - SSH extension, run **Remote-SSH: Connect to Host…**, pick `todo-app.repose` and open `/home/dev/todo-app`. **Zed:** open a remote project over SSH with the same host and folder.

The CLI sets this up with one line in `~/.ssh/config`, `Include ~/.ssh/repose/config`. If your `~/.ssh/config` is read-only (managed by Nix or a dotfiles tool), the CLI tells you to add the line yourself.

Your SSH certificate lasts 24 hours. `run`, `attach`, `open` and `cp` renew it. If `ssh` or your editor gets `Permission denied`, run `repose attach` once, detach, and try again.

### Your SSH keys stay on your laptop

Your laptop's ssh-agent is never forwarded to the machine, and `ssh -A` is refused. Nothing running there, an agent or a package's install script, can use your keys, even while you're attached.

Pushes to GitHub still work. When your `gh` login is copied over, git on the machine sends `git@github.com:` and `ssh://git@github.com/` URLs over HTTPS with that login, so `git push` works without changing the remote. For other git hosts, see [Other git hosts](/docs/secrets#other-git-hosts).

## Time zone

`run` and `attach` set the machine's time zone to your laptop's. New shells pick it up. Until the first `run`, a machine uses the time zone on the dashboard's **Settings** page.

## When the machine stops

A stop ends every process, agents included. After the next start, the tmux session has a fresh `shell` window and the agents' windows are gone. To continue a Claude Code conversation, run `claude --resume` in the checkout and pick it.

## Timing

`REPOSE_TIMING=1 repose run` prints how long each step took. A `run` into a running machine with nothing new to sync usually takes about a second; starting a stopped one, about 10.
