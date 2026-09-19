# 07 · cli

## 1. Goal

`repose` is the one static Go binary a developer installs. It turns "I am in
a project directory" into "I am attached to tmux inside that project's guest",
and everything else (start, stop, secrets, config, snapshots) is a thin, exact
mirror of the HTTP API. It never holds platform secrets and never talks to a
host directly; it talks to the API over HTTPS and to guests over SSH through
the gateway.

## 2. Scope: builds

- `cmd/repose` and `internal/cli/` (Go 1.23+, cobra for commands, no
  framework beyond that).
- Every command in `DESIGN.md` §10 with the flags, output and exit codes in
  §5 below.
- Logto login: authorization code with PKCE and a loopback redirect, device
  code fallback, token storage per `interfaces/cli-config.md`.
- Project resolution from the git remote with normalisation and the
  `projects.json` cache.
- SSH certificate issue and silent refresh, `~/.ssh/repose/` files, the one
  `Include` line in `~/.ssh/config`.
- The `run` sequence: create if new, start if stopped, stream the build,
  certificate, git-based sync with dirty-tree refusal, credential file sync,
  tmux attach or prompt send.
- `open <port>` (SSH `-L`) and `open --desktop` (noVNC on 6080 through the
  same forward, opens the browser).
- `status` table and `--json` for every read command.
- SSE build log rendering with the Nix error and fragment line highlighted.
- `install.sh` and release builds for darwin/arm64, darwin/amd64,
  linux/amd64, linux/arm64, plus `nix run`.
- Shell completion for bash, zsh, fish (`repose completion <shell>`).
- `internal/fakes/api` if it does not exist yet (the consumer writes the
  fake).

### Added by I-8: `events` and `notify`

- `repose events [--since 24h] [--follow]`: prints the project's events from
  `GET /projects/:id/events` as `ts  agent  kind  summary`, one per line;
  `--follow` polls every 10 s. Exit 4 if the project does not exist.
- `repose notify set --email on|off --ntfy <url>|none`: `PATCH /me` with the
  notify block; prints the resulting settings.
- `repose notify test`: `POST /me/notify-test`; prints per-channel `ok` or
  the error string. Exit 1 if every channel failed.

### Binary name collision (DECISIONS I-15)

Arch Linux ships an unrelated `repose` binary in its official repos. The
installer puts ours in `~/.local/bin` and prepends that to PATH in the shell
rc if it is not already first; when `command -v repose` resolves to another
file after install, the installer prints `another repose is on your PATH at
<path>; ours is at ~/.local/bin/repose` and exits 0. `repose version` prints
`repose <version> (herakraft)` so a user can tell which one answered.

## 3. Scope: does not build

- The API itself, the CA, project routing (05-control-plane-api).
- The gateway (06-gateway-edge). The CLI only writes SSH config that points
  at it.
- Anything inside the guest: tmux session creation, hook wiring, secret
  files (02-guest-base, 04-guestd). The CLI sends `tmux` commands over SSH
  and relies on `interfaces/guest-conventions.md`.
- The dashboard (08-dashboard).
- `repose mcp forward` and `repose browser bridge` (deferred, see
  `DECISIONS.md` R2-11 and `DESIGN.md` §18). The command tree reserves the
  names and prints "not available yet" with a link.
- `repose-admin` (operator tool, lives with 05).

## 4. Interfaces

Owns: `interfaces/cli-config.md`.

Consumes: `interfaces/api.md` (every route), `interfaces/ssh-gateway.md`
(login name, certificate file, known_hosts CA line, ssh config block),
`interfaces/guest-conventions.md` (tmux session and window names, credential
paths, `REPOSE=1`).

## 5. Design detail

### 5.1 Command tree

```
repose login [--no-browser]
repose logout
repose run [PROMPT] [--agent claude|opencode|codex|gemini|pi] [--size small|large|xl]
            [--name NAME] [--stash-remote | --discard-remote] [--no-sync] [--no-attach]
repose attach
repose start
repose stop [--no-snapshot]
repose status [--json] [--watch]
repose open PORT [--local-port N] [--no-browser]
repose open --desktop [--no-browser]
repose secrets set NAME [--from-file PATH] [--from-env]
repose secrets list
repose secrets rm NAME
repose config show [--revisions]
repose config edit
repose config apply [PATH]      # PATH defaults to ./repose.nix if present, else opens editor
repose snapshots list
repose snapshots create
repose snapshots restore SNAPSHOT_ID [--as-new NAME]
repose destroy [--yes]
repose logs [--kind console|build|ops] [--since 1h] [--follow]
repose projects                  # list all, ignores cwd
repose version
repose completion bash|zsh|fish
repose mcp forward ...           # reserved, prints not-available message
repose browser bridge            # reserved, prints not-available message
```

Global flags: `--project ID|SLUG` (or `REPOSE_PROJECT`), `--api-url` (or
`REPOSE_API_URL`), `--json` on read commands, `-v` for debug logging to
stderr.

### 5.2 Login

`repose login`:

1. Discover Logto's OIDC config from `config.toml` `logto_issuer` (default
   `https://auth.herakraft.co`, overridable). Cache the discovery document
   for 24 hours.
2. If a browser is available (not `--no-browser`, `REPOSE_NO_BROWSER`
   unset, `DISPLAY` or macOS, not inside a guest (`REPOSE=1`)): start a
   listener on `127.0.0.1:0`, build the authorization URL with
   `code_challenge` (S256), `scope=openid offline_access profile email`,
   `resource=https://api.repose.herakraft.co`, open the browser, wait up to
   5 minutes for the callback, exchange the code. Print `Logged in as
   <handle> (<email>)`.
3. Otherwise device code: `POST /oidc/device/auth`, print

   ```
   Open https://auth.herakraft.co/device and enter code ABCD-EFGH
   Waiting...
   ```

   and poll at the returned interval.
4. Store per `interfaces/cli-config.md`: macOS keychain for the refresh
   token (service `repose`, account `<issuer>`), else
   `credentials.json` 0600.
5. Call `GET /me`. If `billing.has_card` is false, print

   ```
   No card on file. Add one at https://repose.herakraft.co/billing before
   the first `repose run`.
   ```

   Exit 0; the run command will fail with `payment_required` (exit 7) until
   a card exists.

Access token refresh happens transparently in the API client: on 401 with
`unauthenticated`, refresh once, retry once. If the refresh fails, exit 3
with `Not logged in. Run \`repose login\`.`

`repose logout` revokes the refresh token at Logto, deletes the stored
tokens, calls `POST /certs/revoke {all:true}`, removes
`~/.ssh/repose/id_ed25519-cert.pub`. It leaves `projects.json` and the ssh
config in place.

### 5.3 Project resolution

Order:

1. `--project` / `REPOSE_PROJECT`: id or slug, resolved via `GET
   /projects` (cache updated).
2. `projects.json` `by_dir[<abs cwd>]` (set when a project was created with
   `--name` in that directory).
3. `git remote get-url origin` in the cwd's repo root, normalised
   (`interfaces/cli-config.md`), then `projects.json[<remote>]`, then `GET
   /projects` filtered by `remote_url`.
4. Nothing found and the command is `run`: create (see 5.5). Nothing found
   and the command is anything else: exit 4 with

   ```
   No repose project for github.com/a/b. Run `repose run` here to create one,
   or pass --project.
   ```

No git remote and no `--name` on `run`: exit 2 with

```
This directory has no git remote. Pass --name NAME to create a project anyway.
```

### 5.4 Certificates and SSH files

`ensureCert(projectIDs)`:

- Read `~/.ssh/repose/id_ed25519-cert.pub`; if present, valid for more
  than 30 minutes, and its principals cover the requested project ids,
  reuse it.
- Else ensure `~/.ssh/id_ed25519` exists (`ssh-keygen -t ed25519 -N ""` with
  a comment `repose` if missing; never touch an existing key), `POST /certs`
  with the public key and the project ids the user has (all of them, so one
  certificate covers every project), write the certificate 0600, and
  `ssh-add` it to the running agent if `SSH_AUTH_SOCK` is set (best effort,
  since the `CertificateFile` line makes ssh work without the agent).
- Write `~/.ssh/repose/known_hosts` with `@cert-authority
  ssh.repose.herakraft.co,10.64.* <host_ca_pub>` from the response.
- Rewrite `~/.ssh/repose/config` with one `Host <slug>.repose` block per
  project (the block in `interfaces/ssh-gateway.md`). Ensure `~/.ssh/config`
  contains `Include ~/.ssh/repose/config` as its first line, inserted once,
  with a comment `# added by repose`. Never rewrite any other line.

The refresh is silent: every command that opens SSH calls `ensureCert`
first. If `POST /certs` returns `rate_limited`, use the existing cert if it
has any validity left and warn.

### 5.5 The run sequence

```
$ repose run
```

1. Resolve the project. If none: `POST /projects {name: <repo basename or
   --name>, remote_url, class: --size or config default_class or large, tz:
   local zone}`. On `payment_required` exit 7 with the billing URL. On
   `conflict` for the name, append `-2`.., ask.
2. `GET /projects/:id`. If `state` is `stopped`, `POST /start` and wait on
   the op. If `building` or the op is a build, open the SSE log and render it
   (5.8). If `error`, print the last op's error and exit 1.
3. `ensureCert`.
4. Wait until `ssh <slug>.repose true` succeeds, up to 60 seconds after the
   API says `running`, polling every 2 seconds, then print `Connected to
   <slug> (<class>, <host region>)`.
5. Sync (unless `--no-sync`):

   a. Local: `git rev-parse HEAD` → `H`; `git status --porcelain` → dirty
      list; `git ls-files --others --exclude-standard` → untracked list.
   b. Remote, in one SSH exec: `cd ~/<slug> && git status --porcelain`. If
      non-empty and neither `--stash-remote` nor `--discard-remote`: exit 6
      with

      ```
      The guest's working tree has uncommitted changes (3 files):
        M src/auth.go
        ?? notes.md
        ...
      An agent may still be working. Re-run with --stash-remote (keeps them in
      `git stash`) or --discard-remote (throws them away), or `repose attach`
      to look first.
      ```

      `--stash-remote` runs `git stash push -u -m "repose run <ts>"`,
      `--discard-remote` runs `git reset --hard && git clean -fd`.
   c. Remote: `git fetch origin` then `git cat-file -e H`. If missing, ask
      `Commit H is not on origin. Push <branch> now? [Y/n]`, push, fetch
      again. `git checkout --detach H` then `git checkout <branch>` if the
      local branch exists on the remote at that commit, else stay detached
      and say so.
   d. Local diff: `git diff HEAD --binary` piped as `git apply --index`
      over SSH, then a tar of the untracked list piped to `tar -x -C ~/<slug>`.
      Skip files over 100 MB with a warning. Respect `sync.exclude`.
   e. Print `Synced: 4 modified, 2 untracked`.
6. Credential sync (unless `--no-sync`): for each row of the table in
   `interfaces/guest-conventions.md`, if the laptop file exists, `tar` it
   over SSH to the guest path, `chmod 0600`. Print one line
   `Credentials: gh, opencode` naming what was copied. Never the Claude
   file, never Gemini's, never SSH keys.
7. If PROMPT given: agent = `--agent` or project `agent_default`. Over SSH:
   `tmux new-window -t <slug> -n <agent> -c ~/<slug> -d '<agent>'` (name
   becomes `<agent>-2` if the window exists), wait until the pane has been
   idle 1 second (`tmux display -p '#{pane_current_command}'` is the agent
   and no output for 1 s), then `tmux send-keys -t <slug>:<window> -l
   '<prompt>'` and `send-keys Enter`. If the agent is `claude` and
   `~/.claude/.credentials.json` is missing in the guest and no
   `CLAUDE_CODE_OAUTH_TOKEN` secret is set, attach instead of sending so the
   user can complete the login, and print

   ```
   Claude Code is not logged in on this guest yet. Finish the login in the
   window that opens, then re-run with your prompt.
   ```
8. Unless `--no-attach`: `exec ssh -t <slug>.repose tmux attach -t <slug>`
   (attaches to the window just created when there was a prompt). The CLI
   process replaces itself with ssh so signals and the terminal behave
   exactly like plain ssh.

`repose attach` is steps 1 (resolve, no create), 3, 4, 8.

### 5.6 stop, start, destroy, resize

- `stop`: `POST /stop {snapshot: !--no-snapshot}`, spinner on the op, then
  `Stopped <slug>. Snapshot <id> (1.2 GB). Disk is still billed; \`repose
  destroy\` to stop that.`
- `start`: `POST /start`, wait, print connected line. Does not sync.
- `destroy`: prints the project, its last snapshot date, and requires typing
  the slug unless `--yes`. Then `DELETE /projects/:id`. Prints `Destroyed.
  Last snapshot kept until <date>; \`repose snapshots restore <id> --as-new
  NAME\` brings it back.`
- Resize is `repose config apply` with `volume_bytes` in the fragment
  header? No: it is its own route, so `repose resize 80G` exists as a
  hidden alias of `POST /resize`; document it in `features/config.md` only.

### 5.7 status

```
$ repose status
todo-app   large   running   2h14m   claude: working   today $0.31   month $12.40
  host eastus/h-01   ip 10.64.0.7   disk 8.1/40 GB   snapshot 6h ago
  sessions 1   tmux clients 1   docker 2
  last event 12m ago: claude completed "ran tests, 3 failures fixed"
```

`--watch` refreshes every 5 seconds. `--json` prints the `Project` object.
`repose projects` prints one line per project in the same first-line
format.

### 5.8 Build log rendering

`GET /projects/:id/ops/:op/log` is SSE. Lines are printed as received with
a dim `nix ›` prefix. On the `done` event with an error, print the error
block from the op (`build_failed` or `eval_failed`) verbatim, then if
`fragment_line` is set, print the fragment with that line marked:

```
error: attribute 'nodejs_25' missing
   at repose.nix:12:5
      11 |   home.packages = with pkgs; [
      12 |     nodejs_25
         |     ^
```

Exit 10. `eval_failed` and `build_failed` are the only errors that print
Nix output; every other API error prints `{code}: {message}`.

### 5.9 open

`repose open 3000`: `ensureCert`, then `ssh -N -L
127.0.0.1:<local>:127.0.0.1:3000 <slug>.repose` in the foreground, print
`http://localhost:3000 → todo-app:3000 (Ctrl-C to stop)`, and open the
browser unless `--no-browser`. `--local-port` defaults to the same port,
falling back to a free port with a message if taken.

`repose open --desktop`: over SSH `systemctl --user start repose-desktop`
(starts Xvfb, the window manager, x11vnc, noVNC on 6080 per
02-guest-base), then forward 6080 and open
`http://localhost:6080/vnc.html?autoconnect=1`. On Ctrl-C, stop the forward
but leave the desktop running; print how to stop it.

### 5.10 secrets, config, snapshots, logs

- `secrets set NAME`: value from `--from-file`, `--from-env` (reads
  `$NAME`), or a hidden prompt. Validates the name regex client-side.
  `PUT /projects/:id/secrets/NAME`. Prints `Set NAME (pushed to running
  guest)` or `(will be delivered at next start)`.
- `config show`: prints the fragment; `--revisions` lists them with status.
- `config edit`: fetches, opens `$EDITOR` on a temp file, on save `PUT` and
  render the build log, then `Applied revision <id>` or the error block. If
  the build reports `kernel_changed`, print `This change needs a reboot;
  run \`repose stop && repose start\` when the agent is idle.`
- `config apply [PATH]`: same with a file; `./repose.nix` default. The file
  is the fragment and is *not* required to be committed; recommend adding
  it to the repo in the message.
- `snapshots list/create/restore` mirror the API. `restore` without
  `--as-new` requires the project stopped and asks for confirmation.
- `logs`: `GET /logs`, `--follow` polls every 2 s with `since`.

### 5.11 Install and release

`install.sh` at `https://repose.herakraft.co/install.sh`: detects
`uname -sm`, downloads `repose_<version>_<os>_<arch>.tar.gz` from the
GitHub release, verifies the sha256 from `checksums.txt`, installs to
`~/.local/bin` (or `/usr/local/bin` with sudo when `--system`), prints the
PATH hint if needed. GoReleaser config in `.goreleaser.yaml`, CGO off, `-s
-w`, version from the tag into `internal/version`. `nix run
github:heracraft/repose#repose` works via the flake's `packages.repose`.

### 5.12 Output rules

Human output to stdout, progress and warnings to stderr, so `--json` and
pipes are clean. Colours only when stdout is a TTY. No spinner when not a
TTY. All timestamps local.

## 6. Failure modes

| Situation | Outcome |
|---|---|
| Not logged in or refresh failed | exit 3, `Not logged in. Run \`repose login\`.` |
| No card on file on create/start | exit 7, `Add a card at https://repose.herakraft.co/billing first.` |
| `capacity` from create/start | exit 8, `No capacity right now; try again in a few minutes. (We have been alerted.)` |
| Guest stopped on `attach`/`open` | exit 5, `todo-app is stopped. Run \`repose start\`.` |
| Dirty remote tree | exit 6, message in 5.5 |
| Build or eval error | exit 10, Nix error block, fragment line marked |
| SSH cannot connect within 60 s after `running` | exit 1, `Guest is running but SSH did not answer in 60s. \`repose logs --kind console\` may show why.` |
| Gateway rejects certificate | re-issue once; if still rejected exit 1 with the gateway banner verbatim |
| `~/.ssh/config` unwritable | exit 1 naming the file; nothing partial written (write temp + rename) |
| Browser cannot open | print the URL and continue |
| API unreachable | exit 1, `Cannot reach api.repose.herakraft.co: <err>`; never retried more than 3 times with backoff |
| Rate limited on `/certs` | reuse existing cert if valid, warn on stderr |
| Prompt given but agent not installed in guest | exit 1, `Agent 'pi' is not in this guest's config. Add it with \`repose config edit\`.` |
| Second prompt while agent window exists | new window `<agent>-2`, stderr warning `Another claude window is open; two agents share one working tree.` |

## 7. Testing

- Unit: remote normalisation table, project resolution order, ssh config
  rendering (golden files), certificate validity logic, exit codes, SSE
  parser, error block rendering.
- Integration against `internal/fakes/api`: full `run` sequence with a
  local sshd in a container standing in for the guest (Docker test fixture
  `test/guest-sshd/` with tmux and git, trusting the test CA). Covers dirty
  tree refusal, stash and discard, push prompt, credential copy, prompt
  send, second window naming.
- Real: on the M2 host, a second person's laptop, every command, recorded
  in `docs/workstreams/STATUS.md`.
- Cross-platform: the GoReleaser matrix builds all four targets in CI;
  `install.sh` is run in CI on ubuntu and macos runners.

## 8. Rollback

The CLI is stateless on the platform. Rolling back a release is publishing
the previous tag; `install.sh --version X` installs a specific version. The
only files it owns on a laptop are under `~/.config/repose/` and
`~/.ssh/repose/` plus the one `Include` line; `repose logout --purge`
removes all of them including the `Include` line.

## 9. Checklist

- [ ] Every command and flag in 5.1 exists with that name. Evidence: `repose
      --help` tree pasted, diffed against 5.1.
- [ ] Login works with a browser (PKCE loopback) and without
      (`--no-browser`, device code), against the real Logto. Evidence:
      recording or transcript of both.
- [ ] Tokens stored per `interfaces/cli-config.md`; on macOS the refresh
      token is in the keychain and absent from disk. Evidence: `cat
      credentials.json` on macOS shows no refresh token.
- [ ] Remote normalisation passes the table test with at least: ssh, https,
      https with `.git`, uppercase host, trailing slash, `ssh://git@host/`.
      Evidence: test file.
- [ ] `ensureCert` reuses a valid certificate, refreshes an expiring one,
      re-issues when a new project is added. Evidence: unit test with a
      clock.
- [ ] `~/.ssh/config` gets exactly one `Include` line, first line, and no
      other line changes on repeated runs. Evidence: golden test with a
      pre-existing config.
- [ ] `ssh <slug>.repose` works from a plain terminal with no CLI involved
      after one `repose run`. Evidence: transcript.
- [ ] Sync: dirty remote refused with exit 6; `--stash-remote` stashes and
      the stash is listed; `--discard-remote` discards; commit not on origin
      prompts and pushes; untracked files respecting gitignore arrive;
      binary diff applies. Evidence: integration test output.
- [ ] Credential sync copies exactly the four rows and never the Claude,
      Gemini or SSH key files, even if present. Evidence: integration test
      that plants all of them and asserts.
- [ ] Prompt send: window named after the agent, second one `-2`, prompt
      arrives after the TUI is idle (not typed into a shell). Evidence:
      `tmux capture-pane` in the integration test shows the prompt inside
      the agent UI.
- [ ] Claude not-logged-in path attaches instead of sending. Evidence:
      integration test.
- [ ] `attach` execs ssh (the CLI process is replaced). Evidence: `ps`
      shows no `repose` parent during a session.
- [ ] Build log SSE renders, error block matches 5.8 with the marked line.
      Evidence: golden test with a fake eval error.
- [ ] `open PORT` and `open --desktop` work, browser opens, Ctrl-C leaves
      the desktop running. Evidence: transcript on a real guest.
- [ ] `status`, `projects`, `secrets`, `config`, `snapshots`, `logs`,
      `destroy` each round-trip against the real API. Evidence: transcript.
- [ ] Every row of the failure table in §6 is triggered and prints the
      exact message and exit code. Evidence: table of outputs in the PR.
- [ ] `--json` output on every read command is valid JSON with nothing else
      on stdout. Evidence: `repose status --json | jq .` in CI.
- [ ] GoReleaser builds four targets; `install.sh` installs on ubuntu and
      macos CI runners and `repose version` prints the tag. Evidence: CI
      run link.
- [ ] `nix run .#repose` works. Evidence: CI job.
- [ ] Completion scripts generate and load without errors in bash, zsh,
      fish. Evidence: CI job sourcing each.
- [ ] `internal/fakes/api` covers every route the CLI calls. Evidence: the
      fake's route table diffed against `interfaces/api.md`.
- [ ] No secret, token, certificate or prompt text is ever logged at `-v`.
      Evidence: reviewer grepped log calls; test asserts on captured
      output.
- [ ] `features/run-and-attach.md`, `features/sync-at-launch.md`,
      `features/ports-and-previews.md` match the built behaviour. Evidence:
      re-read and diffed by the implementer.
- [ ] `ops/RUNBOOK.md` has entries for: user cannot log in, certificate
      rejected, SSH timeout after running. Evidence: entries exist.
