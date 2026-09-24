---
title: Ports and localhost
description: Reaching a dev server on the machine from your laptop's browser.
section: Using repose
order: 14
---

Every port a program on the machine starts listening on appears on your laptop's `localhost` within about a second, for as long as you're attached with `repose run` or `repose attach`. There's nothing to configure.

Start a dev server in the tmux session:

```
pnpm dev
```

tmux shows a message for a few seconds:

```
⇄ localhost:5173 → :5173
```

and the right side of the status bar lists everything forwarded:

```
⇄ 3000 5173 │ ...
```

Open `http://localhost:5173` on your laptop. Because the address is `localhost`, the browser treats it as a secure context, cookies set for `localhost` work, and OAuth providers that only allow `localhost` redirect URIs accept it. It behaves like running the server on your laptop.

## What gets forwarded

A port is forwarded when a program on the machine listens on it on `127.0.0.1`, `0.0.0.0`, `::1` or `::`, and the port number is 1024 or higher. The forward is removed within two seconds of the program closing the port.

Not forwarded:

- Ports below 1024.
- The desktop's own ports, 5900, 6080 and 6081 (`repose open --desktop` handles those).
- A server listening only on another address, such as a Docker bridge or the machine's own IP. A container's port is forwarded when Docker publishes it on the host (`-p 8080:80`), since that listens on `0.0.0.0`. A port that exists only inside a Docker network isn't.

## When the port is taken on your laptop

If something on your laptop already uses the port, the forward goes to the next free one, and the message says so:

```
⇄ localhost:3001 → :3000 (3000 is taken on your laptop)
```

Port 1355 is portless's proxy port, and the URLs portless prints contain it, so a remap there gets its own message:

```
1355 is taken on your laptop (portless?); todo-app's portless is on localhost:1356
```

If no free port can be found, the CLI tries again after 30 seconds.

## Two laptops at once

Each attached laptop forwards to its own `localhost`. The status bar shows every port forwarded by any attached laptop. The forwards end when you detach.

## Turning it off

```
REPOSE_NO_FORWARD=1 repose attach
```

Set it in your shell profile to keep it off. `repose open` still works.

## `repose open`

For one port, in the foreground, without attaching:

```
$ repose open 3000
http://localhost:3000 → todo-app:3000 (Ctrl-C to stop)
```

It opens the URL in your browser too, unless you pass `--no-browser`. The forward lasts until you press `Ctrl-C` or close the terminal. If the local port is taken, the CLI picks another and prints `port 3000 is taken; forwarding to 3001 instead`. Choose the local port yourself with `--local-port 8080`.

Use `repose open` when you want a forward without a tmux session open, or when automatic forwarding is off.

## Plain SSH

The machine is an SSH host, so the usual flags work too:

```
ssh -L 9229:localhost:9229 todo-app.repose
```

This is handy for a debugger port or a database you don't want forwarded all the time. See [Editors and SSH](/docs/ssh).

## Sharing with someone else

There are no public URLs for a project's ports yet. Everything on the machine is reachable only through your own SSH connection. To show someone a running app, deploy it somewhere, or use a tunnel service such as `ngrok` or `cloudflared` from the machine. Anything you expose that way is public, so treat it accordingly.
