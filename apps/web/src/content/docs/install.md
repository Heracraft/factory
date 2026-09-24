---
title: Install and log in
description: Installing the CLI, logging in, updating and removing it.
section: Start here
order: 3
---

## Requirements

- macOS or Linux, on x86-64 or ARM64. On Windows, install and run the CLI inside WSL.
- `git` and the OpenSSH client (`ssh`, `scp`). Both ship with macOS and with most Linux distributions.
- A GitHub account to sign in with.

You don't need tmux, Nix or Docker on your laptop. Those run on the machine.

## Install

```
curl -fsSL https://repose.herakraft.co/install.sh | sh
```

The script downloads the newest release from GitHub, checks it against the release's `checksums.txt`, and installs it as `~/.local/bin/repose`. If `~/.local/bin` isn't on your `PATH`, it appends an `export PATH=...` line to `~/.zshrc`, `~/.bashrc` or `~/.profile` (whichever matches your shell) and prints what it did. Open a new terminal afterwards.

To install for every user in `/usr/local/bin` instead (the script uses `sudo` if it has to):

```
curl -fsSL https://repose.herakraft.co/install.sh | sh -s -- --system
```

To install one particular release:

```
curl -fsSL https://repose.herakraft.co/install.sh | sh -s -- --version v0.1.9
```

Arch Linux has an unrelated package that also installs a command called `repose`. If the script finds another `repose` earlier on your `PATH`, it prints both locations. Put `~/.local/bin` first, or call ours by its full path.

Check what you have:

```
repose version
```

## Log in

```
repose login
```

The CLI prints a URL and a short code. Open the URL in a browser on any device, sign in with GitHub, and type the code. You don't need a browser on the machine you're logging in from, so this also works over SSH.

The terminal then prints `Logged in as <handle> (<email>)`. If your account has no card yet, it adds a line pointing you at the billing page.

The login is stored in `~/.config/repose/credentials.json`. On macOS the long-lived part of it goes into the login keychain instead, and that file holds only the account's issuer. You stay logged in until you run `repose logout`.

## Update

Run the install command again. It replaces the binary in place. Your login, SSH setup and project list are left alone.

## What the CLI writes on your laptop

| Path                                | What it holds                                                                                                                              |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `~/.config/repose/credentials.json` | Your login (mode 0600).                                                                                                                    |
| `~/.config/repose/projects.json`    | A cache mapping checkouts to projects. Safe to delete; it's rebuilt from the server.                                                       |
| `~/.config/repose/config.toml`      | Optional settings you write yourself. See [Configuration files](/docs/cli-config).                                                         |
| `~/.ssh/repose/`                    | The CLI's own SSH key pair, the short-lived certificate for it, a `known_hosts` file, and an SSH config with one `Host` block per project. |
| `~/.ssh/config`                     | One added line: `Include ~/.ssh/repose/config`.                                                                                            |

The CLI never reads, uses or changes your own keys in `~/.ssh/id_*`. It makes a separate key in `~/.ssh/repose/` and uses that for everything.

It writes nothing into your repositories. No dotfile, no git hook, no git config key.

## Log out and uninstall

```
repose logout
```

This revokes your SSH certificates on the server, deletes the local login and the current certificate, and leaves the rest of the SSH setup in place. `ssh <project>.repose` stops working until you log in again and run a command that fetches a new certificate.

To remove everything the CLI put on your laptop:

```
repose logout --purge
rm ~/.local/bin/repose
```

`--purge` deletes `~/.ssh/repose/`, `~/.config/repose/` and the `Include` line in `~/.ssh/config`. Your projects keep running on the server. Stop or destroy them first if you don't want to keep paying for them.
