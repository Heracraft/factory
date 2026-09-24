---
title: Agents
description: The five agents on every machine, how each one logs in, and how to let them run without asking.
section: Using repose
order: 13
---

Every machine comes with five coding agents installed:

| Agent       | Command    | `--agent`  | How it reports back           |
| ----------- | ---------- | ---------- | ----------------------------- |
| Claude Code | `claude`   | `claude`   | Hooks: finished, needs input  |
| Codex CLI   | `codex`    | `codex`    | Hook: finished                |
| opencode    | `opencode` | `opencode` | Plugin: finished, needs input |
| Gemini CLI  | `gemini`   | `gemini`   | Idle detection                |
| pi          | `pi`       | `pi`       | Idle detection                |

They're the upstream programs, unmodified. repose adds a small hook to each one's configuration so that it can [notify you](/docs/notifications), and keeps your own hooks alongside it. The platform updates the agents when it updates the machine's base; `repose status --json` shows the base version a project is on.

`repose run "prompt"` uses the project's default agent, which is Claude Code unless you set [`default_agent`](/docs/cli-config) before creating the project. Pick another for one prompt with `--agent`:

```
repose run --agent codex "port the build scripts to bun"
```

You can also start any of them by hand in a tmux window, the same way you would on your laptop.

## Logging in

Each agent keeps its own login on the machine's disk. It survives stops and restarts, and it's included in snapshots.

### Claude Code

Claude Code's credentials are never copied from your laptop, and repose never stores them. Anthropic's terms require each user to sign in with their own credentials on an unmodified Claude Code, and a copied refresh token stops working within hours anyway.

Log in once per machine:

1. `repose run` (or `repose attach`) to get a shell on the machine.
2. Type `claude`. It shows a URL.
3. Open the URL on your laptop and approve.
4. Copy the code the page shows and paste it into the terminal.

That's it for the life of the project. If you run `repose run "some prompt"` before logging in, the CLI opens the Claude window for you to log in, and asks you to re-run the prompt afterwards.

A subscription login done this way keeps every Claude Code feature, including Remote Control, which lets you check on the session from the Claude app on your phone.

**Setup token instead.** If you'd rather not log in on each machine, create a long-lived token on your laptop and store it as a secret for the project:

```
claude setup-token
repose secrets set CLAUDE_CODE_OAUTH_TOKEN
```

Paste the token when asked. Claude Code on the machine uses it from the next shell or window it starts. A setup token works without any login on the machine, but Remote Control, connectors and Claude in Chrome don't work with it.

**API key.** To use an Anthropic API key, store it the same way: `repose secrets set ANTHROPIC_API_KEY`.

### Codex CLI and opencode

If you're logged in on your laptop, `repose run` copies the login file over (`~/.codex/auth.json`, `~/.local/share/opencode/auth.json`). You'll see them named on the `Credentials:` line. A login you did on the machine itself is never overwritten by an older one from the laptop.

You can also log in on the machine directly by running `codex` or `opencode` there.

### Gemini CLI

Gemini's OAuth login isn't copied. Use an API key:

```
repose secrets set GEMINI_API_KEY
```

Or run `gemini` on the machine and log in there.

### pi

pi uses a model provider's API key. Store it as a secret under the name the provider's tools expect, such as `ANTHROPIC_API_KEY` or `OPENAI_API_KEY`.

## Letting an agent run without asking

The machine is there so an agent can work unattended, but no agent starts in its no-questions mode unless you tell it to. An agent waiting on a permission prompt sends a "needs input" notification (Claude Code and opencode) or goes quiet until you attach.

**Claude Code.** Put this in `~/.claude/settings.json`, on your laptop to carry it to every machine, or on one machine only:

```json
{
	"permissions": {
		"defaultMode": "bypassPermissions"
	}
}
```

Claude Code may ask you to confirm this mode the first time it starts with it. Attach and accept, and it won't ask again on that machine. You can also switch modes inside a session with `Shift-Tab`.

**Codex CLI.** In `~/.codex/config.toml` on the machine:

```toml
approval_policy = "never"
sandbox_mode = "danger-full-access"
```

For the others, see each agent's own documentation.

## Your Claude Code settings come with you

`repose run` and `repose attach` copy your laptop's Claude Code setup to the machine:

- `~/.claude/CLAUDE.md`
- `~/.claude/settings.json`, merged into the machine's copy (details below)
- `skills/`, `agents/`, `commands/` and `output-styles/`
- `keybindings.json`
- scripts under `~/.claude/` that your `settings.json` hooks or status line run

These never leave your laptop: your login (`.credentials.json`), conversation history (`projects/`, `history.jsonl`), todos, `~/.claude.json`, installed plugin files, and anything named like a key or credential (`.env*`, `*.pem`, `*.key`, SSH keys).

The settings merge works like this. The machine's `settings.json` is the base, and your laptop's goes on top, so any key you set on the laptop wins, `model` included. Permission allow, deny and ask lists are combined, so "don't ask again" answers you gave on the machine survive the next run. Paths under your laptop's home directory are rewritten to `/home/dev`. A hook or status line whose command doesn't exist on the machine (a macOS-only command like `afplay`, a Homebrew path) is left out, and the CLI names it once. These keys are removed before the file leaves your laptop, because they tend to hold secrets: `env`, `apiKeyHelper`, the AWS and GCP auth helpers, `otelHeadersHelper` and `forceLoginMethod`. The previous version on the machine is kept as `settings.json.repose-prev`.

Plugins you enabled on the laptop are installed on the machine in the background from their marketplaces. tmux shows a message saying which ones installed or failed. A plugin from a marketplace that lives in a local directory on your laptop can't be installed there and is skipped with a note.

Only files that changed since the last copy are sent.

## MCP servers

Two browser MCP servers are registered in Claude Code on every machine: `playwright` and `chrome-devtools`. See [Browser](/docs/browser).

HTTP and SSE servers (Linear, Sentry, Notion, GitHub, Stripe and similar) work the same on the machine as anywhere. So do stdio servers that wrap an API and only need `npx` and a token. Store the token as a secret and refer to it as `${VAR}` in the server's config. Add servers on the machine with `claude mcp add`, or commit them to the repository's `.mcp.json`, which syncs with your code.

`~/.claude.json` isn't copied, because its per-project entries are keyed on paths that only exist on your laptop.

Servers that need your laptop can't work on the machine: Apple Notes, Xcode, iMessage, desktop automation, Claude in Chrome, and filesystem servers pointed at laptop paths. `repose mcp forward` is reserved for bridging these while the laptop is open, but it isn't available yet.

## What the agent can do on the machine

The agent runs as the user `dev`, which has passwordless `sudo` and is in the `docker` group. It can install packages, start services, change system files for the running machine, and use the network. It can't reach your laptop or your other projects. See [Security](/docs/security).
