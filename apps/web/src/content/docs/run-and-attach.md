---
title: Run and attach
description: Starting agents, the tmux session, and getting back to it.
section: Using repose
order: 11
---

## `repose run`

`repose run` does everything needed to get you onto the machine, in order:

1. Finds the project for this checkout, or creates it.
2. Starts the machine if it's stopped, or waits for it if it's still building.
3. Gets a fresh SSH certificate if the current one is close to expiring.
4. Copies your tool logins and settings, then your git state and uncommitted work. See [What gets synced](/docs/sync).
5. With a prompt, starts an agent in a new tmux window and types the prompt into it.
6. Attaches your terminal to the machine's tmux session.

Running it twice in a row is safe. The second run finds nothing new to copy and goes straight to attaching.

Without a prompt, you land in whatever tmux window was active last. With a prompt:

```
repose run "migrate the date handling to Temporal and fix the tests that break"
```

You don't need quotes. Everything after the flags is the prompt, so this works too:

```
repose run fix the flaky upload test
```

### Flags

| Flag               | What it does                                                                                                                                                                                                    |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--agent NAME`     | Which agent gets the prompt: `claude`, `codex`, `opencode`, `gemini` or `pi`. Default: the project's, which is `claude` unless [`default_agent`](/docs/cli-config) said otherwise when the project was created. |
| `--no-attach`      | Start the agent and return to your laptop's shell without attaching. Useful in scripts.                                                                                                                         |
| `--no-sync`        | Skip the git sync and the copying of tool logins. Your git and Claude Code settings are still copied, as with `attach`.                                                                                         |
| `--stash-remote`   | If the machine's checkout has uncommitted changes, stash them there before syncing.                                                                                                                             |
| `--discard-remote` | If the machine's checkout has uncommitted changes, throw them away before syncing.                                                                                                                              |
| `--size SIZE`      | `small`, `large` or `xl`, for a project being created. Ignored otherwise.                                                                                                                                       |
| `--name NAME`      | Project name, for a directory with no remote or a second project for the same repository.                                                                                                                       |
| `--project NAME`   | Run against a named project instead of this checkout's.                                                                                                                                                         |

`--stash-remote` and `--discard-remote` exist because the sync refuses to overwrite work on the machine that your laptop doesn't know about. [What gets synced](/docs/sync) explains when that happens.

## How an agent is started

With a prompt, the CLI opens a tmux window named after the agent (`claude`, `codex` and so on) in your checkout directory, starts the agent's normal interactive interface there, waits until it's ready for input, types your prompt and presses Enter. Then it attaches you to that window.

This is the same interface you'd get running `claude` yourself, with its full history and any permission prompts visible when you attach. Nothing runs in a headless mode.

If a window for that agent already exists, the new one is called `claude-2`, then `claude-3`. The CLI warns you:

```
Another claude window is open; two agents share one working tree.
```

Two agents editing the same checkout can step on each other. If that matters, have one of them work in a separate `git worktree` on the machine.

The first time you send a prompt to Claude Code on a machine where it isn't logged in, the CLI opens the window and prints:

```
Claude Code is not logged in on this guest yet. Finish the login in the window that opens, then re-run with your prompt.
```

[Agents](/docs/agents) covers logins for all five agents.

## The tmux session

Each machine has one tmux session, named after the project, that exists from the moment the machine boots. Its first window is `shell`, in `/home/dev/<project>`. Agents get their own windows next to it.

The keys you'll need (press `Ctrl-b`, let go, then the key):

| Keys               | Action                                                  |
| ------------------ | ------------------------------------------------------- |
| `Ctrl-b` `d`       | Detach. Everything keeps running.                       |
| `Ctrl-b` `w`       | Pick a window from a list.                              |
| `Ctrl-b` `n` / `p` | Next / previous window.                                 |
| `Ctrl-b` `c`       | New window with a shell.                                |
| `Ctrl-b` `[`       | Scroll back (then arrow keys or Page Up, `q` to leave). |

The mouse works: click a window name in the status bar to switch, scroll with the wheel to go back through output. tmux keeps 50,000 lines per pane. Text you copy in tmux goes to your laptop's clipboard in terminals that support it (iTerm2, Kitty, WezTerm, Alacritty, Windows Terminal; in macOS Terminal it doesn't).

Detaching never stops anything. Closing the terminal window, losing Wi-Fi and putting the laptop to sleep all count as detaching.

## `repose attach`

```
repose attach            # this checkout's project
repose attach todo-app   # any project, from anywhere
```

`attach` connects to the session without touching the checkout on the machine. In the background it still copies your git and Claude Code settings when they've changed (git settings only when you run it inside the checkout). Use it to check on an agent, or from a second computer where your checkout is older than the machine's. It doesn't start a stopped machine; it tells you the state and which command to run.

Several terminals can be attached at once, from one laptop or several. They all see the same windows. tmux sizes each window to fit the smallest attached terminal, so a narrow terminal shrinks the view for everyone.

## Time zone

`run` and `attach` set the machine's time zone to your laptop's. A change prints a line like `Time zone set to Asia/Tokyo.` New shells and new agent windows use the new zone. Shells that were already open keep the old one until you open a new one.

## When the machine stops

Stopping a machine ends every process on it, including agents in the middle of a task. After `repose start`, the tmux session is there again with a fresh `shell` window, and the agents' windows are gone. Claude Code can pick up where it was interrupted: start it in the checkout with `claude --resume` and choose the conversation.

## Exit codes

`run` and `attach` use the same codes as every other command. The useful ones: `3` not logged in, `5` the machine isn't running (from `attach`), `6` the machine has uncommitted changes and the sync refused, `7` no card on file, `8` no capacity right now, `10` the environment build failed. [Troubleshooting](/docs/troubleshooting) lists them all with the fix for each.
