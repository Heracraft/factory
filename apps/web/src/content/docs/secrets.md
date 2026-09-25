---
title: Secrets and security
description: API keys, tool logins and git settings, where each one lives, and what can reach what.
section: Using repose
order: 14
---

## Store an API key

```
$ repose secrets set STRIPE_SECRET_KEY
Value for STRIPE_SECRET_KEY (not shown):
Set STRIPE_SECRET_KEY (pushed to running guest)
```

Or read the value from a file or from your laptop's environment:

```
repose secrets set GOOGLE_CREDENTIALS --from-file ./service-account.json
repose secrets set OPENAI_API_KEY --from-env
```

On the machine, each secret is an environment variable in new shells and agents, and a file at `/run/repose/secrets/NAME`. Both are kept in memory only: never on the machine's disk, never in snapshots. A change reaches a running machine within seconds; programs already running keep the old value until restarted (`exec $SHELL` in a shell).

```
repose secrets list
repose secrets rm STRIPE_SECRET_KEY
```

`list` shows names and dates, never values. Nothing shows a value again after you set it. Secrets belong to one project; [`repose fork`](/docs/lifecycle#fork-a-project) gives each copy the project's secrets as they are at the time. Names are uppercase letters, digits and underscores, start with a letter and are up to 64 characters; values up to 64 KB.

The dashboard's project **Secrets** page does the same.

## Logins copied from your laptop

At each `repose run`, these are copied straight to the machine over SSH if you have them. repose never stores them.

| Tool       | File                                                                                                        |
| ---------- | ----------------------------------------------------------------------------------------------------------- |
| GitHub CLI | `~/.config/gh/hosts.yml` (with the token, even if it's in the macOS keychain)                               |
| Codex CLI  | `~/.codex/auth.json`                                                                                        |
| opencode   | `~/.local/share/opencode/auth.json`                                                                         |
| Vercel CLI | `~/Library/Application Support/com.vercel.cli/auth.json` (macOS), `~/.local/share/com.vercel.cli/auth.json` |

Your SSH keys never reach the machine, and your ssh-agent isn't forwarded. With `gh` logged in on your laptop, git on the machine sends every GitHub URL, `git@github.com:owner/repo` and `ssh://git@github.com/owner/repo` included, over HTTPS with that login. An agent can push to an SSH remote without any change to it. If you log in to `gh` on the machine instead, run `gh auth setup-git` there once.

Never copied: SSH private keys, Claude Code's login, Gemini's OAuth login. See [Agents](/docs/agents#log-in) for those.

### Other git hosts

GitLab, Bitbucket and your own server have no login that repose copies, and your SSH keys stay on your laptop. Pick one of these.

**A token, kept as a secret.** Create a token with write access to the repository on the git host, then store it:

```
repose secrets set GITLAB_TOKEN
```

On the machine, tell git to use it for that host, and to send the host's SSH URLs over HTTPS:

```
git config --global credential.https://gitlab.com.helper '!f() { echo username=oauth2; echo "password=$GITLAB_TOKEN"; }; f'
git config --global url.https://gitlab.com/.insteadOf git@gitlab.com:
```

Bitbucket takes your username and an app password or access token in the same place. The token is in memory on the machine only, like every secret.

**A deploy key made on the machine.** On the machine:

```
ssh-keygen -t ed25519 -N '' -f ~/.ssh/id_ed25519 -C todo-app.repose
cat ~/.ssh/id_ed25519.pub
```

Add the public key to that one repository as a deploy key with write access. It can push to that repository and nothing else. The private key lives on the machine's disk, so it's in its snapshots; delete the deploy key on the git host when you destroy the project.

## Git and Claude Code settings

Your global git settings are copied, minus credential helpers, signing, URL rewrites, proxies, `core.sshCommand`, `core.hooksPath`, diff and merge tools, a pager or editor the machine doesn't have, and anything that looks like a token. Settings you make on the machine win. Commits made on the machine are unsigned, since the signing key stays on your laptop.

Your Claude Code setup is copied too: `CLAUDE.md`, `settings.json` (with `env` and API key helpers removed), skills, agents, commands and the scripts your hooks run. [Agents](/docs/agents#your-claude-code-setup-comes-along) has the details.

## What an agent on the machine can reach

- **Everything on the machine**, including your checkout, `.env` files and the logins above. It has `sudo`.
- **The internet**, outbound, with the limits in [Limits](/docs/limits).
- **Your git host**, with whatever credentials the machine has.
- **Not your other projects.** Each is a separate machine, and the network stops them from reaching each other.
- **Not your laptop.** The machine can't open connections to it, and it never gets your SSH keys or your ssh-agent. While you're attached, ports on the machine appear on your laptop's `localhost`.

## What repose stores

- Your account (GitHub login, email), and each project's name, git remote, size, state, configuration and build logs.
- Named secrets, encrypted with a key for your account that is itself protected by a key in Azure Key Vault.
- Once a minute, for billing and abuse detection: whether the machine is running, CPU, memory, network and disk use, and the names of running processes. Never their arguments or environment.
- Agent events: which agent, what kind of event, and the agent's own one-line summary.

Never stored: your code, prompts, terminal contents, files on the machine, or the logins copied from your laptop. The [privacy policy](/privacy) has the full list.

Disks aren't encrypted per project yet, so an operator with root on a server can read the disks on it. Every operator login and command inside a machine is audited, and operators look inside only for an incident, an abuse report or a support request you made. Machines and snapshots are in Azure East US.
