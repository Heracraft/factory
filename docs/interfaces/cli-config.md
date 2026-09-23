# CLI files on the laptop

All under `~/.config/repose/` (respecting `$XDG_CONFIG_HOME`), mode 0700.

| File | Contents |
|---|---|
| `credentials.json` | `{refresh_token, access_token, expires_at, logto_issuer, logto_client_id}` (`logto_client_id` since v0.1.1; absent means the built-in id); on macOS the refresh token goes to the keychain (`repose` service) and this file holds only the issuer. Mode 0600. |
| `projects.json` | cache: `{"<normalised remote>": {"project_id", "slug", "name"}, ...}` plus `{"by_dir": {"<abs path>": "project_id"}}` for `--name` projects with no remote, keyed by the repository root (else the directory). `by_dir` is written only when `run` creates such a project there, never for an explicitly named one; an entry is used only when its project's `remote_url` equals the directory's remote (or both are empty), and a mismatched or vanished entry is deleted (DECISIONS I-152). Regenerable from `GET /projects`. |
| `config.toml` | `api_url` (default prod), `gateway`, `default_class`, `default_agent`, `sync.exclude` (extra gitignore-style patterns), `logto_issuer` (default the owner's Logto), `logto_client_id` (default the App ID of the `repose-cli` application there; DECISIONS I-99). |

`~/.ssh/repose/` (0700) holds `id_ed25519` and `id_ed25519.pub` (the
CLI's own key pair, generated without a passphrase, private half 0600;
DECISIONS I-149), `id_ed25519-cert.pub` (the current certificate, for
that key), `known_hosts`, `config` (see ssh-gateway.md), and `cm-*`, the
ControlMaster sockets of open multiplexed connections. The CLI never
creates, reads or changes `~/.ssh/id_*`. In `~/.ssh/config` it owns one
`Include ~/.ssh/repose/config` line before the first `Host` or `Match`
line; a symlinked config is edited at its target, or, when that is read
only, left alone with a message saying where to add the line (I-151).

Remote URL normalisation: strip scheme and `git@`, replace `:` after host
with `/`, strip trailing `.git`, lowercase the whole result (DECISIONS
I-73: not just the host, so the worked example below actually holds).
`git@github.com:A/B.git` and `https://github.com/a/b` both become
`github.com/a/b`.

Environment overrides: `REPOSE_API_URL`, `REPOSE_PROJECT` (project id or
slug, same as `--project`), `REPOSE_NO_BROWSER=1` (forces device code; device code is the default since v0.1.2, `--browser` asks for the loopback PKCE flow, DECISIONS I-101), `REPOSE_NO_FORWARD=1` (no automatic port forwards while attached, DECISIONS I-199; `repose open` still works). `REPOSE_SESSION` is internal: the session helper's options (DECISIONS I-206), never set by hand.

`repose cp [-r] SRC DST` (DECISIONS I-201): one side is `PROJECT:PATH`
or `:PATH` (this checkout's project), the other a laptop path; a path
starting with `/` or `.` is always local, as with scp. A relative guest
path is taken from `~/<slug>`. It runs `scp` with the project's ssh
target (the multiplexed `<slug>.repose` alias), so it refreshes the
certificate like `run`, needs a running guest (exit 5 otherwise), and
exits with scp's code.

Exit codes: 0 ok; 1 generic; 2 usage; 3 not logged in; 4 project not found;
5 guest not running; 6 dirty remote tree (sync refused); 7 payment required;
8 capacity; 10 build failed (Nix error printed); 130 interrupted (Ctrl-C).
Usage covers cobra's own refusals too: an unknown command or flag, a
wrong number of arguments, and two different projects named at once
(DECISIONS I-155).

Output: results on stdout; progress (phase lines, or one spinner line
on a terminal), warnings and errors on stderr; `REPOSE_NO_SPINNER=1` or
`TERM=dumb` gives the plain phase lines on a terminal too (I-154).
