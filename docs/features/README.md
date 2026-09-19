# Features

User-facing behaviour, one feature per file. Each doc says what the user sees,
the behaviour and edge cases that must hold (written so they can be turned
into tests), which workstreams it depends on, and what is deferred. When a
feature doc and a workstream doc disagree on user-visible behaviour, the
feature doc wins; on internals, the workstream doc wins.

| Feature | One line | Status |
|---|---|---|
| [projects.md](projects.md) | How a directory becomes a project, `--name`, per-account limits | first release |
| [run-and-attach.md](run-and-attach.md) | `repose run`, `repose attach`, tmux sessions and windows, the agent picker | first release |
| [sync-at-launch.md](sync-at-launch.md) | Git plus the one-shot diff of uncommitted work, refuse-on-dirty | first release |
| [agents.md](agents.md) | The five agents, wrappers, hooks, Claude login, MCP support | first release (`mcp forward` later) |
| [browser.md](browser.md) | Headless Chromium, Playwright MCP, chrome-devtools-mcp, `open --desktop` | first release (`browser bridge` later) |
| [secrets.md](secrets.md) | Synced tool logins and named secrets | first release |
| [config.md](config.md) | The menu, the Nix fragment, apply, base bumps, hold | first release |
| [snapshots.md](snapshots.md) | Nightly and on-stop snapshots, list, restore | first release |
| [notifications.md](notifications.md) | Completed and needs-input events by email and ntfy | first release (Telegram, Discord later) |
| [ports-and-previews.md](ports-and-previews.md) | `repose open <port>` now, preview URLs later | first release / later |
| [stop-start-destroy.md](stop-start-destroy.md) | Lifecycle states, what each one costs, retention | first release |
| [status-and-logs.md](status-and-logs.md) | `repose status`, `repose logs`, the dashboard view | first release |

Deferred features with a written design live in the doc for the nearest
first-release feature (preview URLs in ports-and-previews.md, `mcp forward` in
agents.md, `browser bridge` in browser.md). Teams have no doc because there is
no design yet; see `../DECISIONS.md` R5-6.
