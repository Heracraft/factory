---
title: Install
description: Install the CLI, log in, update and uninstall.
section: Start here
order: 2
---

The CLI runs on macOS and Linux, x86-64 or ARM64. On Windows, install it inside WSL. It needs `git` and the OpenSSH client. You don't need tmux, Nix or Docker on your laptop; those run on the machine.

## Install

```
curl -fsSL https://repose.herakraft.co/install.sh | sh
```

The script downloads the latest release, checks its checksum and installs `~/.local/bin/repose`. If `~/.local/bin` isn't on your `PATH`, it adds it to your shell's startup file and tells you. Open a new terminal afterwards.

Other options:

```
# for every user, in /usr/local/bin
curl -fsSL https://repose.herakraft.co/install.sh | sh -s -- --system

# a particular release
curl -fsSL https://repose.herakraft.co/install.sh | sh -s -- --version v0.1.11
```

Arch Linux has an unrelated package that also installs a `repose` command. If the script finds another `repose` earlier on your `PATH`, it prints both locations.

## Log in

```
repose login
```

The CLI prints a URL and a short code. Open the URL in any browser, on any device, sign in with GitHub and enter the code. Because the browser doesn't have to be on the same computer, this also works over SSH.

You stay logged in until you run `repose logout`.

## Update

Run the install command again. It replaces the binary and leaves your login and settings alone.

## Uninstall

```
repose logout --purge
rm ~/.local/bin/repose
```

`--purge` also deletes `~/.config/repose/`, `~/.ssh/repose/` and the `Include` line the CLI added to `~/.ssh/config`. Your projects keep running on the server, so stop or destroy them first. [Files on your laptop](/docs/cli#files-on-your-laptop) lists everything the CLI writes.

The CLI never reads or changes your own keys in `~/.ssh`, and it writes nothing into your repositories.
