---
title: Editors and SSH
description: Using plain ssh, scp, VS Code, Cursor or Zed with a project's machine.
section: Using repose
order: 22
---

Every project is an SSH host called `<project>.repose`. The CLI sets this up the first time you run it, so anything that speaks SSH can reach the machine:

```
ssh todo-app.repose
scp todo-app.repose:~/todo-app/report.html .
rsync -av ./data/ todo-app.repose:~/todo-app/data/
```

You log in as `dev`, in your home directory. The tmux session isn't attached automatically; `tmux attach` gets you there.

## How it works

The CLI writes one `Host` block per project to `~/.ssh/repose/config` and adds a single line to `~/.ssh/config`, above its first `Host` or `Match` line:

```
Include ~/.ssh/repose/config
```

A block looks like this:

```
Host todo-app.repose
  HostName ssh.repose.herakraft.co
  User todo-app.yourhandle
  IdentityFile ~/.ssh/repose/id_ed25519
  CertificateFile ~/.ssh/repose/id_ed25519-cert.pub
  IdentitiesOnly yes
  UserKnownHostsFile ~/.ssh/repose/known_hosts
  ForwardAgent yes
  ServerAliveInterval 30
  ControlMaster auto
  ControlPath ~/.ssh/repose/cm-%C
  ControlPersist 10m
```

The connection goes to repose's SSH gateway, which checks your certificate and relays you to the project's machine. The key in `~/.ssh/repose/` belongs to the CLI and is used for nothing else. Your own keys aren't offered.

If your `~/.ssh/config` is a symlink, the CLI edits the file it points to. If that file is read-only (managed by Nix or a dotfiles tool, say), the CLI leaves it alone and tells you which line to add yourself. Until you do, the CLI's own connections still work, but plain `ssh todo-app.repose` doesn't.

## Certificates expire after 12 hours

Your SSH certificate is valid for 12 hours. `repose run`, `repose attach`, `repose open` and `repose cp` renew it when needed. Other tools can't. If `ssh todo-app.repose` or your editor gets `Permission denied`, run one of those commands once (`repose attach`, then detach) and connect again.

## VS Code and Cursor

1. Install the Remote - SSH extension (Cursor has it built in).
2. Run **Remote-SSH: Connect to Host…** and pick `todo-app.repose` from the list. It's there because of the `Include` line.
3. Open the folder `/home/dev/todo-app`.

The editor installs its server on the machine the first time, which takes a minute. Ports your app opens are forwarded by the editor as usual.

## Zed

Open a remote project over SSH with the host `todo-app.repose`, then open `/home/dev/todo-app`.

## Other editors

Any editor with SSH remote support works if it reads `~/.ssh/config`, including its `Include` lines. If yours doesn't, give it the host `ssh.repose.herakraft.co`, the user from the `User` line above, and the key `~/.ssh/repose/id_ed25519` with its certificate `~/.ssh/repose/id_ed25519-cert.pub`.

## Agent forwarding

The generated block sets `ForwardAgent yes`, so your laptop's ssh-agent is available on the machine while you're connected. That lets a `git push` over SSH work with keys that never leave your laptop. It also means that, while you're connected, any process on the machine can ask your agent to sign with the keys loaded in it. It can't read the keys, and it loses access when you disconnect.

The CLI rewrites the block, and there's no setting to turn forwarding off yet. If you don't want it, don't keep keys in your ssh-agent while attached, or use an SSH agent that asks you to approve signing requests, such as 1Password's or Secretive. An agent working alone on the machine should push with an HTTPS token; see [Secrets and logins](/docs/secrets).

## Port forwarding with plain SSH

```
ssh -N -L 5432:localhost:5432 todo-app.repose
```

`repose attach` does this automatically for every port. See [Ports and localhost](/docs/ports).
