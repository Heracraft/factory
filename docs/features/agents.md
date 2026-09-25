# Agents

Five coding agents ship in every guest: Claude Code, opencode, Codex CLI,
Gemini CLI, and pi. The platform wraps each so it reports to the notification
system and lands in the right tmux window; otherwise they are the upstream
binaries, unmodified.

## What the user sees

```
$ repose run "write tests for the payment module"
Starting claude in window todo-app:claude
```

First use of Claude in a guest with no credentials:

```
$ repose run "write tests for the payment module"
claude is not logged in on todo-app. Complete the login in the window that
opens (paste the code from your browser), then rerun this command.
```

Setting the headless fallback:

```
$ claude setup-token                       # on the laptop
$ repose secrets set CLAUDE_CODE_OAUTH_TOKEN
Enter value: ********
Stored. claude on todo-app will use it on next start (no Remote Control,
connectors, or Claude in Chrome with a setup token).
```

Picking a default:

```
$ repose config set agent codex
```

## The five agents

| Agent | Binary | Window | Hook mechanism | Login in guest |
|---|---|---|---|---|
| Claude Code | `claude` | `claude` | `Notification` and `Stop` hooks in `~/.claude/settings.json` calling `repose-hook` | `claude` prints a paste code over SSH; or `CLAUDE_CODE_OAUTH_TOKEN` named secret |
| opencode | `opencode` | `opencode` | its plugin/event hook if present in the shipped version; otherwise tmux pane-idle heuristic | `~/.local/share/opencode/auth.json` synced from the laptop |
| Codex CLI | `codex` | `codex` | `notify` config entry pointing at `repose-hook` | `~/.codex/auth.json` synced from the laptop |
| Gemini CLI | `gemini` | `gemini` | tmux pane-idle heuristic | `GEMINI_API_KEY` named secret |
| pi | `pi` | `pi` | its hooks if present in the shipped version; otherwise pane-idle heuristic | provider API key as a named secret |

The exact hook mechanism per agent is verified when the overlay is built and
recorded in `workstreams/02-guest-base.md`; the table is the intent. The
payload each agent actually sends is recorded as a fixture under
`internal/guestd/hooks/testdata/<agent>/`, one file per shape, so a change in
an agent's payload shows up as a failing test rather than as a notification
that stops arriving. The two agents with no hook have a README there instead,
naming the heuristic test that covers them. Where an
agent has no completion hook, guestd watches the pane: an agent window whose
pane has produced no output for 90 seconds and whose foreground process is
the agent itself is reported `idle`, which becomes a `completed` event once,
until the pane produces output again. That heuristic is named as such in the
notification (`claude finished` versus `gemini went idle`), so the user
knows what they are being told.

## Wrappers

Each binary is wrapped by `nix/overlay/agents/wrap.nix` to:

1. Write or patch its hook configuration so completion and needs-input
   events go to `repose-hook`, which POSTs to `/run/repose/hooks.sock`.
   The patch is idempotent and preserves the user's other hooks.
2. Export `TERM=tmux-256color` and `COLORTERM=truecolor` so the TUIs render.
3. Exec the real binary with all arguments.

`repose-hook` always exits 0. A hook that fails must never block an agent,
because a blocked agent is a silently wasted night.

## Claude Code starts without permission prompts (DECISIONS I-250)

The guest's baseline `~/.claude/settings.json` (from
`/etc/repose/claude-settings.json`) carries
`permissions.defaultMode: "bypassPermissions"` and
`skipDangerousModePermissionPrompt: true`, so Claude Code in a guest runs
commands and edits without asking and shows no warning dialog, including
in `repose run`. The guest is the blast radius and a
snapshot restores it; `dev` is not root, which bypass mode requires.

- The Claude wrapper's setup adds both keys to an existing file only when
  it has no `permissions.defaultMode`; a value the user set, in the guest
  or carried from the laptop by the I-196 merge (laptop on top), is never
  changed. Opting out is setting another `defaultMode`; deleting the key
  brings the default back at the next start.
- These are user settings, never managed settings
  (`/etc/claude-code/managed-settings.json`), which would outrank the user.
- Deny rules, explicit ask rules and removals of critical paths still
  prompt or block in this mode (Claude Code's own rules).
- Codex, opencode, Gemini CLI and pi keep their own defaults; the user docs
  show Codex's two keys.

## The machine guide (DECISIONS I-243)

Every agent is told what the machine offers: that the laptop is out of
reach, that its servers' ports reach the laptop's `localhost` while the user
is attached, how to install a tool now and how the user keeps it, Docker and
databases, secrets (and never to print them), the browser, how to reach the
user, git over HTTPS, and the limits. The one source is
`nix/guest/base/agent-guide.md`; `nix/guest/base/agent-guide.nix` renders it
(comments dropped, a `needs: CMD` line kept only when the guest has `CMD`)
and installs it where each agent reads global instructions without touching
a file the user owns:

| Agent | Where | Mechanism |
|---|---|---|
| Claude Code | `/etc/claude-code/CLAUDE.md` | managed memory, loaded before the user's `~/.claude/CLAUDE.md` |
| Codex | `/etc/codex/config.toml` `developer_instructions` | system config layer; a user-level `developer_instructions` replaces it |
| opencode | `/etc/opencode/opencode.json` `instructions` | managed config dir; `instructions` arrays are unioned with the user's |
| Gemini CLI | `~/.gemini/extensions/repose-machine-guide` → `/etc/repose/gemini-extension` | extension context file, linked by the wrapper; no system-level context exists and `@` imports outside the workspace are refused |
| pi | `~/.pi/agent/extensions/repose-machine-guide.js` → `/etc/repose/pi-extension.js` | extension adding a system prompt section, linked by the wrapper |

The user's `CLAUDE.md`, `AGENTS.md` and `GEMINI.md` are never edited, and
the carried `~/.claude/CLAUDE.md` (I-196) stays the user's. A new guest
capability updates the guide in the same commit; `internal/cli/agent_guide_test.go`
fails when a section of the machine, agents or limits page has no guide
line, and the `guest-agent-guide` VM test checks every agent sends it.

## Versions

Agents come from the platform overlay (`nix/overlay/agents/`), which
repackages upstream binary releases and is bumped by a scheduled job that
opens a pull request with the version diff. Users see the version in `repose
status --verbose` and in the base changelog. nixpkgs is the fallback only;
it lags Claude Code by weeks and Gemini CLI by months (DECISIONS R3-19).

## Claude login and the policy behind it

Anthropic's terms for hosted use of Claude Code require the binary to be
unmodified and every user to authenticate with their own credentials; third
party reuse of subscription OAuth is forbidden. So:

- The platform never stores, proxies, or copies Claude credentials.
  `~/.claude/.credentials.json` is not in the synced-files list and a test
  asserts it never appears in a tar stream. Copied refresh tokens also do
  not refresh, so copying would break within hours anyway.
- The default path is logging in inside the guest. The first `run` with
  agent `claude` and no credentials starts `claude` in the agent window; it
  prints a URL and expects a code pasted back. That keeps every feature,
  including Remote Control, which is the "check on it from my phone"
  feature this product wants.
- The fallback is `claude setup-token` on the laptop, stored as the named
  secret `CLAUDE_CODE_OAUTH_TOKEN`. It works headless and survives guest
  restarts, but loses Remote Control, connectors and Claude in Chrome. The
  CLI says so when the secret is set.
- The wrapper does not modify the binary. Hooks are configuration.

## Your Claude Code config comes with you (DECISIONS I-196)

`run` and `attach` carry the laptop's own Claude configuration into the
guest, and never its login or its history:

- Carried: `~/.claude/CLAUDE.md`, `settings.json` (merged, below),
  `skills/`, `agents/`, `commands/`, `output-styles/`, `keybindings.json`,
  and the scripts under `~/.claude/` that `settings.json` runs (hooks,
  `statusLine`). Files are copied onto what the guest has, so a skill made
  in the guest stays; a directory over 32 MB is skipped with one line.
- Never carried: `.credentials.json` (anywhere under `~/.claude`),
  `projects/` (transcripts), `history.jsonl`, `todos/`,
  `shell-snapshots/`, `file-history/`, `paste-cache/`, `sessions/`,
  `plugins/`, `statsig/`, and `~/.claude.json`. The list above is an
  allowlist, so nothing else is read.
- `settings.json` is merged in the guest with `jq`, never overwritten:
  the guest's file is the base and the laptop's goes on top (the laptop
  wins on any key it sets, `model` included); `permissions.allow`, `deny`
  and `ask` are unioned, so "Yes, and don't ask again" in the guest
  survives the next run; the platform's `repose-hook` entries are removed
  from both sides and added back once, last; the laptop's home directory
  is rewritten to `/home/dev`; and a hook or `statusLine` whose command
  the guest cannot run (`afplay`, a Homebrew path) is dropped and named
  once. The result is checked with `jq empty` before it replaces the old
  file, which is kept as `settings.json.repose-prev`. A laptop file that
  is not JSON is not sent (one warning); a guest file that is not JSON is
  left alone (one warning).
- Plugins: `settings.json` names them but Claude Code does not reinstall
  them on a new machine, so the guest installs the marketplace plugins
  in `enabledPlugins` that it lacks, in the background, with `claude
  plugin marketplace add` and `claude plugin install`, and says in tmux
  what it installed or could not. A failed install is tried again on the
  next run. A plugin whose marketplace is a directory on the laptop is
  named once and skipped.
- Only what changed on the laptop is sent (one marker per item in the
  guest, DECISIONS I-206).

## Behaviour that must hold

- Every agent starts in its own tmux window named after it, in
  `/home/dev/<slug>`, with the prompt typed once the TUI is ready
  (run-and-attach.md).
- Every agent's completion and needs-input reach the API as events within
  10 seconds of the hook firing, or within 90 seconds for heuristic agents.
- A missing login produces the agent's own login prompt in the window plus
  the CLI message above; it never produces a crash loop.
- `repose status` shows per agent window: `working`, `idle`,
  `needs_input`, or `unknown`, from guestd's `AgentState` notifications.
- Removing an agent from the overlay is a base bump with a changelog line;
  a project holding base updates keeps the old one.

## MCP

What works unchanged: HTTP and SSE MCP servers (Notion, Linear, Sentry,
GitHub, Stripe and the like), and stdio servers that are thin wrappers over
an API and only need `npx` and a token. Playwright MCP and chrome-devtools-mcp
are preinstalled (browser.md). Tokens go in as named secrets and are
referenced with `${VAR}` in the MCP config.

What does not work: servers bound to the laptop, such as Apple Notes, Xcode,
iMessage, desktop automation, Claude in Chrome, and filesystem servers
pointed at laptop paths. They are unsupported in the first release
(DECISIONS R2-14). The CLI does not sync `~/.claude.json` because its
project-scoped entries are keyed on absolute laptop paths that do not exist
in the guest; a user who wants an MCP in the guest adds it there, or in the
repo's `.mcp.json`, which syncs with the repo.

### Planned: `repose mcp forward NAME`

For when the laptop is open and a laptop-bound server is wanted anyway:

1. The CLI reads the server's definition from the laptop's Claude config.
2. It starts the stdio server locally wrapped by `mcp-proxy` as HTTP on a
   free local port.
3. It opens an SSH reverse tunnel (`-R`) from a guest port to that local
   port for the life of the CLI process.
4. It registers `NAME` in the guest's Claude user-scope config as an HTTP
   server at `http://127.0.0.1:<port>/mcp`, and removes it when the tunnel
   closes.

It only works while the laptop is up, which is the case this product exists
to escape, so it is a convenience, not a promise.

## Depends on

Workstreams 02 (overlay, wrappers, `repose-hook`), 04 (hook socket,
pane-idle heuristic, AgentState), 05 (events ingest), 07 (`--agent`,
`config set agent`, secrets), 13 (delivery).

## Deferred

`repose mcp forward`. Syncing `~/.claude.json` with path rewriting. Agents
beyond the five (DECISIONS R2-11: anything nixpkgs does not package is a
package the platform maintains).
