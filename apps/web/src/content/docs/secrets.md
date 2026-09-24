---
title: Secrets and logins
description: API keys, tool logins and git settings, and where each one lives.
section: Using repose
order: 17
---

A machine gets credentials in three ways, depending on what they are.

| What                                                           | How it gets there               | Where repose keeps it                                        |
| -------------------------------------------------------------- | ------------------------------- | ------------------------------------------------------------ |
| API keys and tokens you name (`DATABASE_URL`, `STRIPE_KEY`)    | `repose secrets set`            | Encrypted in repose's database, and in memory on the machine |
| Logins your laptop already has (`gh`, Codex, opencode, Vercel) | Copied by `repose run` over SSH | Nowhere. They go laptop to machine.                          |
| Claude Code                                                    | You log in on the machine       | Nowhere. See [Agents](/docs/agents).                         |

## Named secrets

```
$ repose secrets set STRIPE_SECRET_KEY
Value for STRIPE_SECRET_KEY (not shown):
Set STRIPE_SECRET_KEY (pushed to running guest)
```

The value isn't echoed as you type. To read it from somewhere else:

```
repose secrets set GOOGLE_CREDENTIALS --from-file ./service-account.json
repose secrets set OPENAI_API_KEY --from-env
```

`--from-env` reads the variable of the same name from your laptop's environment.

On the machine, each secret is:

- an environment variable in every new shell, and in every agent started after it was set;
- a file, `/run/repose/secrets/NAME`, readable only by `dev`.

Both live in memory (a tmpfs), so they're gone the moment the machine stops and are written again at the next start. They're never on the machine's disk and never in snapshots.

A change reaches a running machine within a few seconds. Programs that were already running keep the old value until you restart them; open a new tmux window or run `exec $SHELL` to pick it up in a shell. If the machine is stopped, the CLI says `(will be delivered at next start)`.

```
$ repose secrets list
DATABASE_URL        2026-09-17 14:02
STRIPE_SECRET_KEY   2026-09-23 09:41

$ repose secrets rm STRIPE_SECRET_KEY
Removed STRIPE_SECRET_KEY
```

Nothing, including the dashboard, ever shows a secret's value again after you set it. `list` shows names and dates only.

Rules:

- Names are uppercase letters, digits and underscores, starting with a letter, up to 64 characters.
- Values can be up to 64 KB, and binary is fine.
- Secrets belong to one project. The same name in two projects is two separate secrets.

The dashboard's Secrets page (from a project's page) does the same: add, list and delete.

### How they're protected

Values are encrypted with a key specific to your account, which is itself encrypted by a key held in Azure Key Vault. The API never returns a value once stored. Secrets don't appear in logs, events or notifications. Before a build log is stored, every current secret value of the project is searched for and replaced with `[redacted]`, and a Nix configuration that contains a secret's value is refused before it's built.

Don't put secrets in your [Nix configuration](/docs/config). Anything in it ends up in the machine's Nix store, which every user on the machine can read.

## Logins copied from your laptop

At each `repose run`, these files are copied from your laptop into the same place on the machine, with mode 0600, if they exist:

| Tool       | File on your laptop                                                                                                   |
| ---------- | --------------------------------------------------------------------------------------------------------------------- |
| GitHub CLI | `~/.config/gh/hosts.yml`                                                                                              |
| Codex CLI  | `~/.codex/auth.json`                                                                                                  |
| opencode   | `~/.local/share/opencode/auth.json`                                                                                   |
| Vercel CLI | `~/Library/Application Support/com.vercel.cli/auth.json` on macOS, `~/.local/share/com.vercel.cli/auth.json` on Linux |

The run prints which ones went: `Credentials: gh, codex`. A file is copied again only when it changed on your laptop, so a rotated token arrives at the next run. If the machine's copy is newer (because you logged in there), the machine's is kept and the CLI says so.

If your `gh` token lives in the macOS keychain, the copy of `hosts.yml` sent to the machine includes the token so `gh` works there. If `gh` is logged in and the project's remote is on github.com, git on the machine is also set up to push over HTTPS using `gh`, so an agent can `git push` without your SSH keys.

These files go straight from your laptop to the machine over SSH. repose's servers never store them.

**Never copied**, whatever you have on your laptop: SSH private keys, Claude Code's `.credentials.json`, Gemini's OAuth file.

### Other git hosts

For GitLab, Bitbucket or a self-hosted server, give the machine a token. For example, with a GitLab personal access token stored as a secret:

```
repose secrets set GITLAB_TOKEN
```

then on the machine:

```
git config --global credential.helper '!f() { echo username=oauth2; echo "password=$GITLAB_TOKEN"; }; f'
```

The machine's `~/.gitconfig` is on its disk, so this lasts for the life of the project.

Your laptop's ssh-agent is forwarded to the machine for as long as you're attached, so a push over SSH works then too. It stops working when you detach, which is why an HTTPS token is the better choice for an agent working on its own.

## Git settings

Your laptop's global git settings travel too, from the checkout's point of view, so an `includeIf` that sets a work email for this directory is applied. They're written to `~/.config/git/repose-carried` on the machine and included first from `~/.gitconfig`, so anything you set on the machine itself wins.

Left out: credential helpers, SSH and URL rewrite settings, commit signing, proxies, hooks paths, merge and diff tools, and any value that looks like a token or password. A setting that points at a laptop path the machine doesn't have, or an editor or pager it doesn't have, is dropped with a note. Your global gitignore (`core.excludesFile`) travels as its contents.

Commit signing isn't carried because the signing key stays on your laptop. Commits the agent makes on the machine are unsigned.

## `.env` files

Gitignored `.env` and `.env.*` files in the checkout are copied with the code. See [What gets synced](/docs/sync#your-code). They land on the machine's disk and so are included in snapshots, the same as your code.
