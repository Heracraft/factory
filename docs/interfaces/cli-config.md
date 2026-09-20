# CLI files on the laptop

All under `~/.config/repose/` (respecting `$XDG_CONFIG_HOME`), mode 0700.

| File | Contents |
|---|---|
| `credentials.json` | `{refresh_token, access_token, expires_at, logto_issuer, logto_client_id}` (`logto_client_id` since v0.1.1; absent means the built-in id); on macOS the refresh token goes to the keychain (`repose` service) and this file holds only the issuer. Mode 0600. |
| `projects.json` | cache: `{"<normalised remote>": {"project_id", "slug", "name"}, ...}` plus `{"by_dir": {"<abs path>": "project_id"}}` for `--name` projects. Regenerable from `GET /projects`. |
| `config.toml` | `api_url` (default prod), `gateway`, `default_class`, `default_agent`, `sync.exclude` (extra gitignore-style patterns), `logto_issuer` (default the owner's Logto), `logto_client_id` (default the App ID of the `repose-cli` application there; DECISIONS I-99). |

`~/.ssh/repose/` holds `id_ed25519-cert.pub` (the current certificate),
`known_hosts`, and `config` (see ssh-gateway.md).

Remote URL normalisation: strip scheme and `git@`, replace `:` after host
with `/`, strip trailing `.git`, lowercase the whole result (DECISIONS
I-73: not just the host, so the worked example below actually holds).
`git@github.com:A/B.git` and `https://github.com/a/b` both become
`github.com/a/b`.

Environment overrides: `REPOSE_API_URL`, `REPOSE_PROJECT` (project id or
slug, same as `--project`), `REPOSE_NO_BROWSER=1` (forces device code).

Exit codes: 0 ok; 1 generic; 2 usage; 3 not logged in; 4 project not found;
5 guest not running; 6 dirty remote tree (sync refused); 7 payment required;
8 capacity; 10 build failed (Nix error printed).
