# Guest conventions

What every guest guarantees, so the CLI, hooks and guestd can rely on it.

## Filesystem

| Path | What |
|---|---|
| `/home/dev/<slug>` | the project checkout; the tmux session's default directory |
| `/home/dev/.factory/project.json` | `{project_id, slug, name, remote_url, user_handle, class, tz}` written by guestd at SetupProject |
| `/run/factory/secrets/<NAME>` | named secret values, tmpfs, 0400 dev |
| `/run/factory/secrets.env` | `export NAME='...'` lines, sourced by login shells |
| `/run/factory/hooks.sock` | hook ingest, HTTP over unix, group dev |
| `/run/factory/guestd.sock` | dev-only stand-in for vsock (absent in real guests) |
| `/etc/factory/base-version` | the platform base version string |
| `/nix/store` | read-only virtio-fs mount of the host store |
| `/var/log/factory/console.log` | not used; console goes to the serial device and hostd captures it |

## tmux

- Server socket is the default for user `dev`.
- Session name = project slug, created at boot with window `shell` in
  `/home/dev/<slug>`.
- Agent windows are named after the agent: `claude`, `opencode`, `codex`,
  `gemini`, `pi`. A second instance gets `claude-2`.
- `factory attach` = `tmux attach -t <slug>`; `factory run "prompt"` =
  `tmux new-window -t <slug> -n <agent> -c /home/dev/<slug> '<agent> ...'`
  then `tmux send-keys -t <slug>:<agent> '<prompt>' Enter` after the TUI is
  up (guestd waits for the pane to be idle 1 s).
- `set -g set-clipboard on`, `set -g mouse on`, `set -g history-limit 50000`.

## Agent wrappers

Each agent binary is wrapped (`nix/overlay/agents/wrap.nix`) to:

1. Ensure its hook config points at `/run/factory/hooks.sock` (Claude Code:
   `~/.claude/settings.json` hooks `Notification` and `Stop` run
   `factory-hook`; others per their doc in `features/agents.md`).
2. Set `TERM=tmux-256color`, `COLORTERM=truecolor`.
3. Exec the real binary.

`factory-hook` reads the hook JSON from stdin, maps it to `{agent, kind,
summary}`, and POSTs to the socket. It exits 0 always so a hook failure never
blocks an agent.

## Credentials the CLI syncs into the guest

| Laptop path | Guest path | Owner/mode |
|---|---|---|
| `~/.config/gh/hosts.yml` | `/home/dev/.config/gh/hosts.yml` | dev 0600 |
| `~/.codex/auth.json` | `/home/dev/.codex/auth.json` | dev 0600 |
| `~/.local/share/opencode/auth.json` | `/home/dev/.local/share/opencode/auth.json` | dev 0600 |
| `git config user.name/email` | `/home/dev/.gitconfig` (those two keys only) | dev 0644 |

Never `~/.claude/.credentials.json`, never `~/.gemini/oauth_creds.json`
(OAuth over SSH is unreliable; Gemini uses `GEMINI_API_KEY` as a named
secret), never SSH private keys.

## Users and privileges

`dev` uid 1000, groups `wheel docker`, `sudo` without password. `root` has
no password and no SSH. `guestd` runs as root. `virtiofs` mount is owned by
root, read-only.

## Environment

`TZ` from `project.json`, `LANG=C.UTF-8`, `EDITOR=nvim` (present),
`FACTORY_PROJECT=<slug>`, `FACTORY=1` (so scripts can detect they are in a
guest), `NPM_CONFIG_PREFIX=/home/dev/.npm-global` (the nodejs store path is
read-only, so `npm i -g` needs a writable prefix), `PATH` includes
`/home/dev/.local/bin`, `/home/dev/.local/share/pnpm` and
`/home/dev/.npm-global/bin`.

## Ports

Anything the user binds on `0.0.0.0` or `127.0.0.1` inside the guest is
reachable through `factory open <port>` (SSH `-L`). Nothing is exposed
otherwise. noVNC listens on 6080 when the desktop is started.
