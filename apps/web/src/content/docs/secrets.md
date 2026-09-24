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

`list` shows names and dates, never values. Nothing shows a value again after you set it. Secrets belong to one project. Names are uppercase letters, digits and underscores; values up to 64 KB.

The dashboard's project **Secrets** page does the same.

## Logins copied from your laptop

At each `repose run`, these are copied straight to the machine over SSH if you have them. repose never stores them.

| Tool       | File                                                                                                        |
| ---------- | ----------------------------------------------------------------------------------------------------------- |
| GitHub CLI | `~/.config/gh/hosts.yml` (with the token, even if it's in the macOS keychain)                               |
| Codex CLI  | `~/.codex/auth.json`                                                                                        |
| opencode   | `~/.local/share/opencode/auth.json`                                                                         |
| Vercel CLI | `~/Library/Application Support/com.vercel.cli/auth.json` (macOS), `~/.local/share/com.vercel.cli/auth.json` |

With `gh` logged in and a github.com remote, git on the machine pushes over HTTPS with that login, so an agent can push without your SSH keys.

Never copied: SSH private keys, Claude Code's login, Gemini's OAuth login. See [Agents](/docs/agents#log-in) for those.

### Other git hosts

For GitLab, Bitbucket or your own server, store a token and tell git on the machine to use it:

```
repose secrets set GITLAB_TOKEN
```

then, on the machine:

```
git config --global credential.helper '!f() { echo username=oauth2; echo "password=$GITLAB_TOKEN"; }; f'
```

## Git and Claude Code settings

Your global git settings are copied, minus credential helpers, signing, URL rewrites and anything that looks like a token. Settings you make on the machine win. Commits made on the machine are unsigned, since the signing key stays on your laptop.

Your Claude Code setup is copied too: `CLAUDE.md`, `settings.json` (with `env` and API key helpers removed), skills, agents, commands and the scripts your hooks run. [Agents](/docs/agents#your-claude-code-setup-comes-along) has the details.

## What an agent on the machine can reach

- **Everything on the machine**, including your checkout, `.env` files and the logins above. It has `sudo`.
- **The internet**, outbound, with the limits in [Limits](/docs/limits).
- **Your git host**, with whatever credentials the machine has.
- **Not your other projects.** Each is a separate machine, and the network stops them from reaching each other.
- **Not your laptop.** The machine can't open connections to it. While you're attached, two things link them: your ssh-agent is forwarded (processes can ask it to sign, not read keys), and ports on the machine appear on your laptop's `localhost`.

## What repose stores

- Your account (GitHub login, email), and each project's name, git remote, size, state, configuration and build logs.
- Named secrets, encrypted with a key for your account that is itself protected by a key in Azure Key Vault.
- Once a minute, for billing and abuse detection: whether the machine is running, CPU, memory, network and disk use, and the names of running processes. Never their arguments or environment.
- Agent events: which agent, what kind of event, and the agent's own one-line summary.

Never stored: your code, prompts, terminal contents, files on the machine, or the logins copied from your laptop. The [privacy policy](/privacy) has the full list.

Disks aren't encrypted per project yet, so an operator with root on a server can read the disks on it. Every operator login and command inside a machine is audited, and operators look inside only for an incident, an abuse report or a support request you made. Machines and snapshots are in Azure East US.
