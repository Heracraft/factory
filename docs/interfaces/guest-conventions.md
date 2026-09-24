# Guest conventions

What every guest guarantees, so the CLI, hooks and guestd can rely on it.
Produced by `nix/guest/base/` (workstream 02); every path, name and variable
here exists in that module under exactly this name.

## Filesystem

| Path | What |
|---|---|
| `/home/dev/<slug>` | the project checkout; the tmux session's default directory |
| `/home/dev/.repose/project.json` | `{project_id, slug, name, remote_url, user_handle, class, tz}` written by guestd at SetupProject |
| `/etc/repose/env` | `TZ=` and `REPOSE_PROJECT=` lines written by guestd at SetupProject, sourced by every shell; the CLI replaces the `TZ=` line (through `sudo`, root 0644, by rename) on `run` and `attach` when the laptop's zone differs (I-198) |
| `/etc/repose/base-version` | the platform base version string (same as `nixos-version`'s label) |
| `/etc/repose/claude-settings.json` | the platform hooks (`Notification`, `Stop` → `repose-hook`) the Claude wrapper merges into `~/.claude/settings.json` |
| `/etc/repose/mcp.json` | the platform MCP servers (`playwright`, `chrome-devtools`, both `--headless`) the Claude wrapper merges into `~/.claude.json` `mcpServers` |
| `/etc/repose/agent-guide.md` | the machine guide agents read, rendered from `nix/guest/base/agent-guide.md` (DECISIONS I-243); the same text is `/etc/claude-code/CLAUDE.md` (Claude Code's managed memory), `developer_instructions` in `/etc/codex/config.toml`, the file named by `instructions` in `/etc/opencode/opencode.json`, and `GEMINI.md` in `/etc/repose/gemini-extension/`; `/etc/repose/pi-extension.js` reads it at each pi run |
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
| `/run/repose/paths-registered` | written by guestd after the first `RegisterPaths`; `repose-paths.service` waits for it (up to 180 s) and `home-manager-dev.service` runs after that (DECISIONS I-67) |
| `/var/lib/repose/paths-loaded` | sha256 of the last registration `nix-store --load-db` took, on the volume; a `RegisterPaths` with the same bytes and `/nix/var/nix/db/db.sqlite` present only writes the stamp above (DECISIONS I-225) |
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
  focus-events on`, `set -g update-environment "DISPLAY SSH_AUTH_SOCK
  SSH_CONNECTION LANG COLORTERM"` (no `TZ`: an attach from a terminal
  without one would clear the session's zone).
- `TZ` in tmux: the global environment's `TZ` is the project's zone, set
  by `repose-tmux-session` from `/etc/repose/env` when it creates the
  session, and set again (global and every session) by the CLI on each
  `run` and `attach` from a laptop in another zone (I-198), so every new
  window and agent starts in the laptop's zone. Shells already running
  keep theirs. The CLI's attach also sends `TZ` on the SSH session
  (`SendEnv`), for bases older than this rule whose `update-environment`
  still lists it.

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
   - `gemini`, `pi`: no hook (guestd's pane-idle heuristic reports for
     them); the machine guide is linked in as an extension (I-243):
     `~/.gemini/extensions/repose-machine-guide` →
     `/etc/repose/gemini-extension`, and
     `${PI_CODING_AGENT_DIR:-~/.pi/agent}/extensions/repose-machine-guide.js`
     → `/etc/repose/pi-extension.js`. A stale link of that name is
     repointed; anything else at the path is left alone.
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
| `~/Library/Application Support/com.vercel.cli/auth.json` (macOS), `~/.local/share/com.vercel.cli/auth.json` (Linux) | `/home/dev/.local/share/com.vercel.cli/auth.json` (the Vercel CLI's login, proposal item 3; new in this release) | dev 0600 |
| `git config user.name/email` | inside the carried git config below (DECISIONS I-195). The old shape, the two keys written straight into `/home/dev/.gitconfig` with `git config --global`, is what a CLI before I-195 still does, and stays accepted: the first carry removes them from `~/.gitconfig` when they equal the carried values, so they cannot shadow later changes | dev 0644 |
| (when gh travelled and the project's remote is on github.com) | `/home/dev/.gitconfig`: `url.https://github.com/.insteadOf git@github.com:` and `credential.https://github.com.helper = !gh auth git-credential`, so the SSH `origin` guestd sets is pushed over HTTPS with gh's login (DECISIONS I-150) | dev 0644 |

When the laptop's gh keeps its token in the system keyring (gh 2.40+),
the `hosts.yml` that travels carries that token as `oauth_token` under
`github.com:`; the laptop's own file is not changed. All of this is one
ssh, before the git steps of the sync.

Never `~/.claude/.credentials.json`, never `~/.gemini/oauth_creds.json`
(OAuth over SSH is unreliable; Gemini uses `GEMINI_API_KEY` as a named
secret), never SSH private keys.

## Laptop config the CLI carries (DECISIONS I-195..I-198, I-206)

In the same ssh as the credentials on `run`, and from the session helper
beside the attach on `attach`. Each item is applied by its own `sh -e`
and ends by writing its marker; a failed item leaves the guest's previous
state and is named once.

| What | Guest path | Notes |
|---|---|---|
| the laptop's `git config --global --list --includes`, run in the checkout, minus the I-195 denylist and the keys that hold a secret (I-211), plus the checkout's own `user.name`/`user.email` | `/home/dev/.config/git/repose-carried` (dev 0644, replaced whole by rename) | `/home/dev/.gitconfig` starts with `[include] path = ~/.config/git/repose-carried`, added once, so the guest's own keys after it win. Path values missing in the guest and a `core.pager`/`core.editor` not on PATH are removed from the file and named once. When `~/.gitconfig` is a symlink (home-manager) nothing is added and the CLI says what to add |
| `core.excludesFile`'s contents | `/home/dev/.config/git/ignore` | git's default excludes file |
| the laptop's zone | `TZ=` in `/etc/repose/env`, tmux global and per-session `TZ` | see "tmux" |
| `~/.claude/CLAUDE.md`, `keybindings.json`, `skills/`, `agents/`, `commands/`, `output-styles/`, and the scripts under `~/.claude` that `settings.json` runs (DECISIONS I-196) | the same paths under `/home/dev/.claude/`, copied onto what is there (`cp -R`, modes kept) | one marker per file or directory (`claude-claude-md`, `claude-skills`, `claude-scripts`, ...) |
| `~/.claude/settings.json` | `/home/dev/.claude/settings.json`, merged by `internal/cli/claude_merge.jq` with the base's `jq`: guest file as the base, laptop's on top (without `env`, `apiKeyHelper`, `aws*`/`gcp*`, `otelHeadersHelper`, `forceLoginMethod`, removed on the laptop, I-211) with its home rewritten to `/home/dev`, `permissions.allow/deny/ask` unioned, `repose-hook` entries stripped from both and `/etc/repose/claude-settings.json`'s appended, hooks and `statusLine` whose command does not resolve dropped. Written as `settings.json.tmp`, checked with `jq empty`, the old file kept as `settings.json.repose-prev`, renamed into place. An invalid guest file is left alone | marker `claude-settings` |
| `enabledPlugins` from a marketplace | installed by `~/.repose/claude-plugins.sh` (started with `setsid -f`, reads `~/.repose/claude-plugins.json`) with `claude plugin marketplace add` / `claude plugin install`, reporting through `tmux display-message` | marker `claude-plugins`, written only when every install worked |
| gitignored `.env` / `.env.*` files in the checkout, outside dependency directories, up to 1 MB (DECISIONS I-197; `run` only) | the same relative path under `/home/dev/<slug>/`, dev 0600, mtime kept; a guest file with a newer mtime is kept (`#kept <path>`) | written at the end of the sync's apply script, after the checkout; marker `env` |
| the laptop's global tools and the project's commands (DECISIONS I-221, I-222; `run` only) | `/home/dev/.repose/tools-wanted.json`, then `repose-tools-install plan` (base, `nix/guest/base/tools-carry.nix`) | see "Tools carry"; marker `tools`, written by the installer when a pass over the list ends |
| markers | `/home/dev/.repose/carry/<item>` | the hash of the laptop input last applied; the probe prints them as `#marker <item> <hash>`, and `#marker tools-notices waiting` while `~/.repose/tools-notices` is non-empty |
| the tool logins' files (DECISIONS I-224) | `/home/dev/.repose/creds-paths`, one expanded path per line | written with marker `creds` after the logins; the probe prints `#credsmissing` when one of them is gone, and the logins are sent again |
| the last completed sync's key (DECISIONS I-224) | `.git/repose-synced-key` in the checkout | hex sha256 of what the laptop sent; emptied before the apply touches the checkout, written at its end; the probe prints `#synckey`, `#head` and `#headref` |

### Tools carry (DECISIONS I-221, I-222)

`~/.repose/tools-wanted.json`, written by the CLI:
`{"v":1, "hash":"<32 hex>", "items":[{"name", "bins":[...], "manager",
"pkg", "version", "from":"laptop"|"project"}], "node":"<major>"}`.
`manager` is `npm`, `pnpm`, `bun`, `go`, `cargo`, `uv`, `pipx` or absent
(nixpkgs only); `pkg` is the manager's name (the Go package path for
`go`); names, versions and commands are restricted to
`[A-Za-z0-9@/._+-]` by the CLI. `node` is absent when the project pins no
single major.

`repose-tools-install plan` (dev, in the carry's ssh, milliseconds):
prints `#installing <name> ...` for the items none of whose `bins` is on
the login PATH and that did not fail before, plus `nodejs_<major>` when
the guest's node is another major and dev's nix profile comes first on
PATH; prints `#warn ...` naming `repose config add nodejs_<major>` when it
does not; writes the missing commands, one per line, to
`$XDG_RUNTIME_DIR/repose-installing`; starts the user unit
`repose-tools-carry` (`--no-block`). A base without the command leaves
the file in place and writes no marker, so the list is sent again after
the base is upgraded.

`repose-tools-carry.service` (user unit of dev, `Nice=10`, idle I/O, also
wanted by `default.target` so a cut-short pass finishes at the next
boot): runs `repose-tools-install run` in a login shell. For each missing
item: the nixpkgs attribute with `bin/<first bin>` (`nix-locate
--minimal --no-group --type x --type s --whole-name --at-root
/bin/<command>`, each line `<attr>.<output>`; the attribute named like the
command first, then one outside a package set, then the shortest; without
nix-locate, the attribute named like the command)
via `nix profile add nixpkgs#<attr>`; else the manager, into the login
PATH (`npm install -g` into `~/.npm-global`, `go install` with
`GOBIN=~/.local/bin`, `cargo install --root ~/.local`, `uv tool install`
for uv and pipx). Each command leaves `repose-installing` when its tool
is done; the file is removed when the pass ends. Output goes to
`~/.repose/tools-install.log` (trimmed at 1 MB); a failure appends
`Could not install <name>: <last output line>` to `~/.repose/tools-notices`,
which the next carry prints as `#warn` lines and deletes, and records the
item in `~/.repose/tools/failed`, so it is not tried again until its
entry changes. The node step records the attribute it added in
`~/.repose/tools/node` and replaces it when the project asks for another
major; it removes what it added when `bash -lc 'node --version'` does not
then report the major.

## Caches (DECISIONS I-202, I-208)

- Docker: `/etc/docker/daemon.json` `registry-mirrors: ["http://10.63.255.254:5000"]`
  and that address in `insecure-registries`. dockerd falls back to
  Docker Hub when the mirror fails; a login or another registry goes
  direct.
- npm, pnpm, yarn (classic): `repose-npm-registry.service`, a user unit of
  `dev` at boot, appends once to `/home/dev/.npmrc`
  `registry=http://10.63.255.254:4873/` (with a comment line naming
  I-202) when the front answers, and records it in
  `/home/dev/.repose/npm-registry` (`added`). A `~/.npmrc` that already
  sets `registry=` or holds a `registry.npmjs.org` token is never
  changed (`own`). With no answer it changes nothing and tries at the next
  boot. A deleted line stays deleted. A project's own `.npmrc` still
  wins, as `~/.npmrc` is below it for npm and pnpm alike.
  `npm_config_registry` is **not** set: npm lets it beat a project's
  `.npmrc`.

## Memory pressure (DECISIONS I-200)

guestd owns `oom_score_adj` for `dev`'s processes and re-applies it every
5 s: -800 for the tmux server (`tmux: server`) and for each agent
window's agent process (the shallowest process in the window's tree whose
name or executable is the agent's binary), and 0 for any other `dev`
process holding a negative value (it inherited the agent's or the tmux
server's on fork: a dev server an agent started, a pane's shell). A
positive value the user set is left alone, and nothing is ever killed or
stopped by guestd. `dev` cannot lower its own value, which is why root
owns this. When the kernel does kill something, guestd's `oom` warning
names it.

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
`PLAYWRIGHT_BROWSERS_PATH=/home/dev/.cache/ms-playwright` (writable;
`repose-playwright-seed.service` links the base's packaged browser
revisions into it at boot, never over a real directory; until I-228 it
was the read-only store path), `PRISMA_ENGINES_MIRROR=http://127.0.0.1:850`
(I-228), `PKG_CONFIG_PATH` naming openssl, zlib, sqlite and libffi (I-228),
`PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS=1`, `PUPPETEER_SKIP_DOWNLOAD=1`,
`PUPPETEER_EXECUTABLE_PATH` and `CHROME_BIN` (the guest's chromium).
`GOPATH=/home/dev/go`, `CARGO_HOME=/home/dev/.cargo`,
`RUSTUP_HOME=/home/dev/.rustup`, `BUN_INSTALL=/home/dev/.bun`,
`DENO_INSTALL_ROOT=/home/dev/.deno`,
`COMPOSER_HOME=/home/dev/.config/composer`,
`GEM_HOME=/home/dev/.local/share/gem` (I-227). `PATH` starts with every
package manager's user bin dir, listed in `nix/guest/base/user-bin-dirs.nix`
(`/home/dev/.local/bin`, `/home/dev/.local/share/pnpm`,
`/home/dev/.npm-global/bin`, `/home/dev/go/bin`, `/home/dev/.cargo/bin`,
`/home/dev/.bun/bin`, `/home/dev/.deno/bin` and the rest), in login and
non-login shells, tmux windows, and dev's systemd user units (I-227). `python`, `python3` and `python3.12` in
`/run/current-system/sw/bin` are a wrapper that adds nix-ld's library
directory to `LD_LIBRARY_PATH` for manylinux wheels and keeps its own
path as `sys.executable` (I-228). `DISPLAY=:99` only while the desktop's X
server socket `/tmp/.X11-unix/X99` exists (checked at every shell start).

## Ports

Anything the user binds on `0.0.0.0` or `127.0.0.1` (or `::`, `::1`)
inside the guest is reachable through `repose open <port>` (SSH `-L`), and
is forwarded automatically while a CLI is attached (DECISIONS I-199), read
with `ss -Hltn` (iproute2, in the base). Platform-owned ports, never
auto-forwarded: 6080, 6081, 5900. Each attached CLI records its forwarded
ports in `/home/dev/.repose/forwards/<id>`; the project session's
`status-right` is set from their union and unset when none is left.
Nothing is exposed otherwise. The desktop listens only on `127.0.0.1`: noVNC on 6080 (the
socket-activated entry point), websockify on 6081, VNC on 5900.
`repose-prisma-engines.socket` listens on `127.0.0.1:850` (under 1024, so
never forwarded) and answers every GET with a redirect to
binaries.prisma.sh, a `linux-nixos` engine path rewritten to
`debian-openssl-3.0.x` (I-228).

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
the runner uses one queue pair. Memory is `shared=on` (virtio-fs needs it);
the volume is opened `direct=on` (O_DIRECT, DECISIONS I-230).
The kernel line gets `ip=<ip>::<gateway>:<netmask>:<hostname>:eth0:off`,
which the guest turns into its static network configuration.
