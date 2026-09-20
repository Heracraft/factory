# Guest conventions

What every guest guarantees, so the CLI, hooks and guestd can rely on it.
Produced by `nix/guest/base/` (workstream 02); every path, name and variable
here exists in that module under exactly this name.

## Filesystem

| Path | What |
|---|---|
| `/home/dev/<slug>` | the project checkout; the tmux session's default directory |
| `/home/dev/.repose/project.json` | `{project_id, slug, name, remote_url, user_handle, class, tz}` written by guestd at SetupProject |
| `/etc/repose/env` | `TZ=` and `REPOSE_PROJECT=` lines written by guestd at SetupProject, sourced by every shell |
| `/etc/repose/base-version` | the platform base version string (same as `nixos-version`'s label) |
| `/etc/repose/claude-settings.json` | the platform hooks (`Notification`, `Stop` → `repose-hook`) the Claude wrapper merges into `~/.claude/settings.json` |
| `/etc/repose/mcp.json` | the platform MCP servers (`playwright`, `chrome-devtools`, both `--headless`) the Claude wrapper merges into `~/.claude.json` `mcpServers` |
| `/etc/repose/agents.json` | `{<agent>: {binary, version, hook}}` for every shipped agent, for `repose status --verbose` |
| `/etc/profile.d/repose.sh` | sources `/etc/repose/env` and `/run/repose/secrets.env`, exports `DISPLAY=:99` while the desktop runs, prepends the user bin dirs to `PATH` |
| `/etc/ssh/principals/dev` | the accepted certificate principals (the project id), written by guestd `SetPrincipals` |
| `/etc/ssh/ssh_host_ed25519_key`, `ssh_host_ed25519_key-cert.pub`, `user_ca.pub` | symlinks to the reserved secrets below (DECISIONS I-35) |
| `/run/repose/` | tmpfs (part of `/run`), 0755 root |
| `/run/repose/ssh_host_ed25519_key`, `/run/repose/ssh_host_ed25519_key-cert.pub`, `/run/repose/user_ca.pub` | the reserved secrets, root 0600 / 0644, written by guestd `WriteSecrets`; a throwaway key is generated at first sshd start when none was delivered yet |
| `/run/repose/secrets/<NAME>` | named secret values, tmpfs, 0400 dev, directory 0700 dev |
| `/run/repose/secrets.env` | `export NAME='...'` lines, 0400 dev, sourced by login shells |
| `/run/repose/hooks.sock` | hook ingest, HTTP over unix, 0660 root:dev, created by guestd |
| `/run/repose/guestd.sock` | dev-only stand-in for vsock (absent in real guests) |
| `/run/repose/desktop/vnc-password` | the noVNC/VNC password for the current desktop start, 0600 dev (DECISIONS I-33) |
| `/run/repose/desktop/last-client` | mtime of the last observed desktop client; the idle stop reads it |
| `/nix/.ro-store` | read-only virtio-fs mount of the host store (tag `ro-store`) |
| `/nix/.rw-store` | the guest's writable store overlay (upper dir `store/`, work dir `work/`), on the thin volume |
| `/nix/store` | overlay of the two: what `nix profile install` in the guest writes lands in `/nix/.rw-store` |
| `/home/dev/.local/state/nix/profiles/profile` | dev's nix profile; `repose-pin-profile` copies its closure into the overlay whenever it changes |
| `/var/log/repose/console.log` | not used; console goes to the serial device and hostd captures it |

## tmux

- Server socket is the default for user `dev`.
- Session name = project slug, created by the user unit
  `repose-tmux-session.service` (started by a path unit once
  `/home/dev/.repose/project.json` exists, and by guestd at SetupProject)
  with window `shell` in `/home/dev/<slug>`. Running it again is a no-op.
- Agent windows are named after the agent: `claude`, `opencode`, `codex`,
  `gemini`, `pi`. A second instance gets `claude-2`.
- `repose attach` = `tmux attach -t <slug>`; `repose run "prompt"` =
  `tmux new-window -t <slug> -n <agent> -c /home/dev/<slug> '<agent> ...'`
  then `tmux send-keys -t <slug>:<agent> '<prompt>' Enter` after the TUI is
  up (guestd waits for the pane to be idle 1 s).
- `/etc/tmux.conf`: `set -g set-clipboard on`, `set -g mouse on`, `set -g
  history-limit 50000`, `set -g default-terminal tmux-256color`, `set -ga
  terminal-overrides ",*:Tc"`, `set -s escape-time 10`, `set -g
  focus-events on`.

## Agent wrappers

Each agent binary is wrapped (`nix/overlay/agents/wrap.nix`) to:

1. Run `repose-agent-setup <agent>`, which makes the agent's hook config
   point at `repose-hook` without touching anything the user configured:
   - `claude`: `~/.claude/settings.json` gains the entries from
     `/etc/repose/claude-settings.json` under `hooks.Notification` and
     `hooks.Stop` unless an entry whose command contains `repose-hook`
     already exists under that event; `~/.claude.json` `mcpServers` gains
     the servers from `/etc/repose/mcp.json`, user entries winning on a
     name clash.
   - `codex`: `~/.codex/config.toml` gains `notify = ["repose-hook"]`
     unless a `notify` key exists.
   - `opencode`: `~/.config/opencode/plugins/repose.js` is installed if
     absent (never overwritten).
   - `gemini`, `pi`: nothing; guestd's pane-idle heuristic reports for them.
2. Set `TERM=tmux-256color` (when inside tmux), `COLORTERM=truecolor`, and
   `REPOSE_HOOK_AGENT=<binary>` so `repose-hook` knows who called it.
3. Exec the real binary with `"$@"`.

`repose-hook` takes the agent from `REPOSE_HOOK_AGENT` or `--agent`
(`REPOSE_AGENT` is accepted for one release, DECISIONS I-58) and the socket
from `REPOSE_HOOK_SOCKET`, `REPOSE_HOOKS_SOCKET` or `--socket`, defaulting to
`/run/repose/hooks.sock`. It reads the hook JSON from stdin (or from
`argv[1]`, which is how Codex's `notify` passes it), maps it to `{agent, kind,
summary, window?}`, POSTs it to the socket, and exits 0 always so a hook
failure
never blocks an agent. Mapping:

| Agent | Payload | kind | summary |
|---|---|---|---|
| claude | `hook_event_name=Stop` | `completed` | last assistant text from the transcript tail (200 chars), else `claude finished` |
| claude | `Notification` with `notification_type` in `permission_prompt`, `idle_prompt`, `agent_needs_input` | `needs_input` | `message` |
| claude | `StopFailure` | `error` | `error` or `message` |
| codex | `type=agent-turn-complete` | `completed` | `last-assistant-message` |
| opencode | plugin sends `{agent, kind, summary}` already mapped: `session.idle` → `completed`, `session.error` → `error`, `permission.updated` or `permission.asked` → `needs_input` | | |
| any | a payload that already has `agent` and `kind` | passed through | |

Anything else is dropped silently. `window` is the tmux window name of the
caller's `$TMUX_PANE` when set.

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

`dev` uid 1000, gid 1000 (group `dev`), groups `wheel docker kvm`, `sudo`
without password, linger enabled (its user manager runs from boot). `root`
has no password and no SSH (`PermitRootLogin no`, `AllowUsers dev`).
`guestd` runs as root. The `virtiofs` mount is owned by root, read-only.
`users.mutableUsers = false`: there is no other account.

## Environment

`TZ` and `REPOSE_PROJECT=<slug>` from `/etc/repose/env`, `LANG=C.UTF-8`,
`EDITOR=nvim` (present), `REPOSE=1` (so scripts can detect they are in a
guest), `COLORTERM=truecolor`, `NPM_CONFIG_PREFIX=/home/dev/.npm-global`
(the nodejs store path is read-only, so `npm i -g` needs a writable
prefix), `PNPM_HOME=/home/dev/.local/share/pnpm`,
`PLAYWRIGHT_BROWSERS_PATH=<store path of playwright-driver.browsers>`,
`PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS=1`, `PUPPETEER_SKIP_DOWNLOAD=1`,
`PUPPETEER_EXECUTABLE_PATH` and `CHROME_BIN` (the guest's chromium).
`PATH` includes `/home/dev/.local/bin`, `/home/dev/.local/share/pnpm` and
`/home/dev/.npm-global/bin`. `DISPLAY=:99` only while the desktop's X
server socket `/tmp/.X11-unix/X99` exists (checked at every shell start).

## Ports

Anything the user binds on `0.0.0.0` or `127.0.0.1` inside the guest is
reachable through `repose open <port>` (SSH `-L`). Nothing is exposed
otherwise. The desktop listens only on `127.0.0.1`: noVNC on 6080 (the
socket-activated entry point), websockify on 6081, VNC on 5900.

## Desktop (DECISIONS I-33)

Units `repose-xvfb.service` (`Xvfb :99`, 1600x1000), `repose-openbox.service`,
`repose-x11vnc.service` (127.0.0.1:5900, password from
`/run/repose/desktop/vnc-password`, regenerated at every start),
`repose-novnc.service` (websockify + noVNC on 127.0.0.1:6081), and
`repose-novnc.socket` on 127.0.0.1:6080 whose proxy service pulls the whole
chain in on the first connection. `repose-desktop-idle-check.timer` runs
every minute and, after 30 minutes without a client on 6081 or 5900,
starts `repose-desktop-idle.service`, which stops all of them;
`systemctl start repose-desktop-idle` stops them now. The user's browsers
and browser MCP servers run in the user slice `repose-browser.slice`
(`MemoryMax` 1.5 GB small, 3 GB large, 6 GB xl).

## `repose-guest-profile`

The script the CLI's `open`, `sync` and the hooks rely on:

- `repose-guest-profile` prints `{project_id, slug, name, dir, tz, class,
  base_version, desktop: {running, display, novnc_port, password_file}}`.
- `repose-guest-profile desktop start` starts the chain and prints the
  password; `desktop stop` stops it; `desktop status` prints `running` or
  `stopped`.

## Runner contract (hostd ⇄ `mkGuestRunner`, DECISIONS I-34)

`nix/flake.nix` exposes `lib.mkGuestRunner { fragmentModule, class,
baseVersion, ... }`, which evaluates the base plus home-manager plus the
fragment for Cloud Hypervisor and returns a package with:

| Path | What |
|---|---|
| `bin/run` | starts cloud-hypervisor for one guest; every per-guest value is an argument |
| `bin/virtiofsd` | starts virtiofsd for the store share with the flags the guest expects |
| `bin/shutdown <api socket>` | presses the virtual power button through the CH API |
| `share/repose/system` | the NixOS toplevel (`system_closure`); `kernel` and `initrd` inside it are what `kernel_changed` compares |
| `share/repose/kernel`, `initrd`, `cmdline`, `base-version`, `class` | what `bin/run` boots, for inspection |

`bin/run --guest-id ID --ip A --gateway A --cid N --volume DEV --tap NAME
--mac MAC [--netmask M] [--vcpu N] [--mem MiB] [--hostname NAME]
[--state-dir DIR] [--api-socket P] [--serial tty|socket|PATH]
[--virtiofs-socket P] [--vsock-socket P] [--extra-cmdline "..."] [-- CH
args...]`. Defaults under `--state-dir` (`/var/lib/repose/guests/<id>`):
`ch.sock` (API), `console.sock` (serial), `virtiofsd.sock` (store share,
must be listening before start), `vsock.sock`. The vsock socket is a unix
socket speaking Cloud Hypervisor's handshake: connect, write `CONNECT
5000\n`, read `OK <port>\n`, then the stream is guestd's. The tap must
exist with `vnet_hdr` (`ip tuntap add NAME mode tap user hostd vnet_hdr`);
the runner uses one queue pair. Memory is `shared=on` (virtio-fs needs it).
The kernel line gets `ip=<ip>::<gateway>:<netmask>:<hostname>:eth0:off`,
which the guest turns into its static network configuration.
