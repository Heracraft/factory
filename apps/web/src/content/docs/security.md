---
title: Security
description: What an agent on the machine can reach, what repose stores, and who can see it.
section: Account
order: 31
---

## What an agent on the machine can reach

Everything on the machine. It runs as `dev`, which has `sudo`. Treat the machine's disk as readable and writable by any agent you run there, including your checkout, `.env` files, logins copied from your laptop and logins made on the machine.

The internet, outbound.

Your git host, with whatever credentials the machine has: your `gh` login if it was copied, and any token you stored as a secret.

Your other projects: no. Each project is its own virtual machine, and network rules stop machines from reaching each other or the server they run on.

Your laptop: no network path. The machine can't open connections to it, and your SSH private keys are never copied to it. Two things do come from your laptop while you're attached:

- **ssh-agent forwarding.** While a session is connected, processes on the machine can ask your laptop's ssh-agent to sign with the keys loaded in it. They can't read the keys, and the access ends when you disconnect. See [Editors and SSH](/docs/ssh#agent-forwarding).
- **Port forwards.** Servers on the machine appear on your laptop's `localhost`, and pages you open from them run in your laptop's browser like any local dev server. Nothing travels the other way: the machine can't reach ports on your laptop.

## What travels from your laptop, and where it goes

| What                                             | Goes to               | Stored by repose |
| ------------------------------------------------ | --------------------- | ---------------- |
| Code, commits, uncommitted changes, `.env` files | The machine, over SSH | No               |
| `gh`, Codex, opencode and Vercel logins          | The machine, over SSH | No               |
| Git and Claude Code settings (secrets stripped)  | The machine, over SSH | No               |
| Names and versions of your global tools          | The machine, over SSH | No               |
| Named secrets you set                            | repose's API          | Yes, encrypted   |
| Your Nix configuration                           | repose's API          | Yes              |

SSH connections pass through repose's gateway, which relays them to the machine and stores nothing of what passes through. The gateway records that a session opened and closed, with the project and certificate serial.

Never copied, whatever is on your laptop: SSH private keys, Claude Code's credentials, Gemini's OAuth file, and settings that look like tokens or passwords. [Secrets and logins](/docs/secrets) has the full rules.

## What repose records

The [privacy policy](/privacy) has the complete list. The main items:

- Your account: GitHub login, email, handle, time zone, notification settings.
- Each project's name, git remote URL, size, state, configuration and build logs.
- Once a minute, for billing and abuse detection: whether the machine is running, CPU and memory use, network bytes, disk use, the number of SSH sessions and tmux clients and Docker containers, and which agents are in which windows and whether they're working.
- Once a minute, the names of the processes running, with their CPU, memory and network use. Never their command-line arguments, environment variables or open files.
- Agent events: agent name, kind of event and the agent's own summary (at most 1 KB).
- The machine's serial console, which carries boot and kernel messages, for 30 days. Your terminals don't write to it.

Never recorded: your prompts, terminal contents, anything agents write beyond the event summaries above, command-line arguments, environment variables, file paths or contents inside the machine, secret values, and the logins copied from your laptop.

## Who can see your data

repose's operators can log in to the servers your machine runs on. Disks aren't encrypted per project yet, so an operator with root on a server can read the disks of machines on it. Every operator login, and every command an operator runs inside a machine, goes into an audit log. Operators look inside a machine only to investigate an incident, an abuse report or a support request you made.

Snapshots are stored in Azure Blob Storage in East US with server-side encryption. The machines run in Azure East US.

## Secrets

Named secrets are encrypted with a key for your account, which is itself encrypted by a key in Azure Key Vault that can't be exported. The API never returns a value after it's set. On the machine they live in memory only and are never written to its disk or to snapshots.

## Your SSH access

Access to a machine is by SSH certificate, never by password. Certificates are issued to the CLI's own key (in `~/.ssh/repose/`), name the projects they're valid for, and expire after 12 hours. `repose logout` revokes them.
