---
title: Configuration files
description: config.toml, environment variables and the files the CLI keeps on your laptop.
section: Reference
order: 42
---

## `~/.config/repose/config.toml`

Optional. The CLI works without it. If `XDG_CONFIG_HOME` is set, the file is under that directory instead.

```toml
# Size for new projects: small, large or xl. Default large.
default_class = "small"

# Agent for `repose run "prompt"` in projects created from now on.
default_agent = "codex"

[sync]
# More gitignore-style patterns to keep off the machine, on top of
# .gitignore and the dependency directories that never travel.
exclude = ["dist", "*.mp4", "fixtures/large"]
```

| Key             | Default  |                                                                                                                                                                                                                          |
| --------------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `default_class` | `large`  | Size for projects created without `--size`.                                                                                                                                                                              |
| `sync.exclude`  | none     | Extra patterns the sync leaves out. A directory name matches at any depth. v0.1.9 and older read only the quoted top-level form, `"sync.exclude" = ["dist"]`; newer versions read both.                                  |
| `default_agent` | `claude` | The agent `repose run "prompt"` uses without `--agent`, for projects created after you set it. Each project keeps the default it was created with. Not in a release yet; with v0.1.9 every project defaults to `claude`. |

Other keys (`api_url`, `gateway`, `logto_issuer`, `logto_client_id`) point the CLI at a different repose installation. Leave them out.

## Environment variables

| Variable              | Effect                                                                                     |
| --------------------- | ------------------------------------------------------------------------------------------ |
| `REPOSE_PROJECT`      | The project to act on, like `--project`.                                                   |
| `REPOSE_NO_FORWARD=1` | Don't forward the machine's ports automatically while attached. `repose open` still works. |
| `REPOSE_NO_SPINNER=1` | Print one line per step, with no animated progress line. `TERM=dumb` does the same.        |
| `REPOSE_TIMING=1`     | Print how long each step of `run` and `attach` took.                                       |
| `REPOSE_NO_BROWSER=1` | Kept for older scripts. Login uses a device code by default anyway.                        |
| `REPOSE_API_URL`      | A different API, like `--api-url`.                                                         |

## Files on your laptop

| Path                                         |                                                                                                                          |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `~/.config/repose/credentials.json`          | Your login, mode 0600. On macOS the long-lived token is in the keychain and this file holds only the issuer.             |
| `~/.config/repose/projects.json`             | Which checkout belongs to which project. A cache; delete it and it's rebuilt.                                            |
| `~/.config/repose/carry-hashes.json`         | Hashes of the Claude Code files last copied, so unchanged files aren't read again. Hashes only, never contents. A cache. |
| `~/.config/repose/config.toml`               | Your settings, above.                                                                                                    |
| `~/.ssh/repose/id_ed25519`, `id_ed25519.pub` | The CLI's own SSH key, without a passphrase, used for nothing else.                                                      |
| `~/.ssh/repose/id_ed25519-cert.pub`          | The current certificate for that key, valid 12 hours.                                                                    |
| `~/.ssh/repose/config`                       | One `Host` block per project. Rewritten by the CLI.                                                                      |
| `~/.ssh/repose/known_hosts`                  | The gateway's host key authority.                                                                                        |
| `~/.ssh/repose/cm-*`                         | Sockets for shared SSH connections, which stay open up to 10 minutes after a command.                                    |
| `~/.ssh/config`                              | One line added: `Include ~/.ssh/repose/config`.                                                                          |

`repose logout --purge` removes all of these.

## Output

Results go to stdout. Progress, warnings and errors go to stderr, so `repose projects --json | jq` works. On a terminal, progress is a single line with a spinner and elapsed time. Elsewhere (a pipe, CI) each step is its own line.
