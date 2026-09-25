---
title: Agents
description: The five agents on every machine, how each logs in, and how to let them run without asking.
section: Using repose
order: 15
---

Every machine has five coding agents installed, unmodified:

| Agent       | `--agent`  | Login                                      | Notifies you          |
| ----------- | ---------- | ------------------------------------------ | --------------------- |
| Claude Code | `claude`   | Log in on the machine, or a setup token    | Finished, needs input |
| Codex CLI   | `codex`    | Copied from your laptop                    | Finished              |
| opencode    | `opencode` | Copied from your laptop                    | Finished, needs input |
| Gemini CLI  | `gemini`   | `GEMINI_API_KEY` secret, or on the machine | When it goes idle     |
| pi          | `pi`       | A provider's API key as a secret           | When it goes idle     |

`repose run "prompt"` uses Claude Code. To use another agent for one prompt:

```
repose run --agent codex "port the build scripts to bun"
```

To change the default for projects you create from now on, set `default_agent = "codex"` in `~/.config/repose/config.toml`. You can also start any agent by hand in a tmux window.

## Let it run without asking

Claude Code starts in `bypassPermissions` mode on every machine: it runs commands and edits files without asking. The machine is the limit of what it can break, and a snapshot can put it back. Your deny rules still apply, and removing a critical path such as a home directory still asks.

To start in another mode, set `defaultMode` in `~/.claude/settings.json`, on your laptop (copied to every machine at `run`) or on one machine. Your value is kept.

```json
{
	"permissions": {
		"defaultMode": "default"
	}
}
```

`default` asks before edits and commands, `acceptEdits` asks before most commands but not edits, `plan` plans first. `Shift-Tab` switches modes inside a session. On a Pro, Max or Team plan, Claude Code may ask once whether to switch to auto mode; answer no to keep bypass.

The other agents ask as they normally do unless you configure them. An agent waiting on a permission prompt sends a "needs input" notification (Claude Code and opencode) or waits until you attach.

**Codex CLI:** in `~/.codex/config.toml` on the machine:

```toml
approval_policy = "never"
sandbox_mode = "danger-full-access"
```

For the others, see each agent's own documentation.

## Let it ask you

Any agent can message you or ask you a question with two commands on the machine: `repose-notify "text"` sends a notification, and `repose-ask --options yes,no "question"` waits for your answer and prints it. Add a line to the agent's instructions (`CLAUDE.md`, `AGENTS.md`) telling it to use them. The answer can come from ntfy, email, the dashboard or `repose reply` on your laptop; see [Notifications](/docs/notifications#agents-can-message-you-and-ask-questions).

## Log in

Logins are kept on the machine's disk. They survive stops and are in snapshots.

**Claude Code.** Its login is never copied from your laptop, so log in once per machine: type `claude`, open the URL on your laptop, approve, paste the code back. If you send a prompt before logging in, the CLI opens the Claude window for the login and asks you to run the prompt again after. A subscription login keeps Remote Control, so you can follow the session in the Claude app.

To skip the per-machine login, store a long-lived token from your laptop as a secret. Remote Control, connectors and Claude in Chrome don't work with it.

```
claude setup-token
repose secrets set CLAUDE_CODE_OAUTH_TOKEN
```

An API key works the same way: `repose secrets set ANTHROPIC_API_KEY`.

**Codex and opencode.** Your laptop's login is copied at each `run`. A login you made on the machine is never overwritten by an older one.

**Gemini CLI.** Store `GEMINI_API_KEY` as a secret, or run `gemini` on the machine and log in there.

**pi.** Store your model provider's key, such as `ANTHROPIC_API_KEY` or `OPENAI_API_KEY`.

## Your Claude Code setup comes along

`run` and `attach` copy from your laptop's `~/.claude/`: `CLAUDE.md`, `settings.json`, `skills/`, `agents/`, `commands/`, `output-styles/`, `keybindings.json`, and scripts your hooks or status line run. Plugins you enabled are installed on the machine in the background.

Your `settings.json` is merged into the machine's: your keys win, and permission lists are combined, so answers you gave on the machine are kept. Keys that tend to hold secrets (`env`, `apiKeyHelper` and the cloud auth helpers) are removed first. Hooks that call commands the machine doesn't have, such as macOS's `afplay`, are left out with a note.

Never copied: your login, conversation history, `~/.claude.json`, and anything named like a key or credential.

## MCP servers

HTTP servers (Linear, Sentry, Notion, GitHub and the like) and stdio servers that only need `npx` and a token work on the machine. Store the token as a secret and refer to it as `${VAR}`. Add servers there with `claude mcp add`, or commit them in the repository's `.mcp.json`.

Servers that need your laptop (Apple Notes, Xcode, desktop automation, Claude in Chrome) don't work on the machine. The browser tools are covered in [The machine](/docs/machine#browser).

## What agents are told about the machine

Every agent on the machine is given a short guide to it: that it's a separate machine and can't reach your laptop, that the ports its servers listen on reach your laptop's `localhost`, how to install a missing tool and how you keep it (`repose config add`), Docker and databases, where your secrets are and never to print them, the browser tools, how to reach you, and the [limits](/docs/limits). To read it, on the machine:

```
cat /etc/claude-code/CLAUDE.md
```

Claude Code reads it as system-wide memory, Codex from `/etc/codex/config.toml`, opencode from `/etc/opencode/opencode.json`, and Gemini CLI and pi as an extension called `repose-machine-guide`. Your own `CLAUDE.md`, `AGENTS.md` and `GEMINI.md` are never changed, and what they say comes on top of the guide. The guide changes only with a platform update.
