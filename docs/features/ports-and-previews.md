# Ports and previews

A dev server running in the guest reaches the user's browser through an SSH
port forward the CLI manages. Public preview URLs per project are designed
here and built later, so the gateway is written with that path in mind.

## What the user sees

While `repose run` or `repose attach` is attached, every port the guest
starts listening on is on the laptop's localhost too, with no command
(DECISIONS I-199). Start `pnpm dev` in the guest and, inside tmux:

```
⇄ localhost:5173 → :5173                          (a message for 4 s)
... ⇄ 3000 5173 │ "dev@izma" 14:02 23-Sep-26         (the status bar's right side)
```

`http://localhost:5173` on the laptop is the guest's vite, a secure
context, an OAuth-friendly `localhost` redirect URI and the same cookies
as local development, which a preview hostname could not give.

The explicit, foreground form stays for one port, for a laptop without
the session helper (Windows), or with auto-forward off:

```
$ repose open 3000
http://localhost:3000 → todo-app:3000 (Ctrl-C to stop)
```

```
$ repose open --desktop
http://localhost:6080/vnc.html?autoconnect=1 (Ctrl-C stops the forward; the desktop keeps running)
```

(browser.md covers the desktop.)

## Behaviour that must hold

Auto-forward (I-199), run by the session helper (run-and-attach.md):

- The helper reads the guest's listeners with `ss -Hltn` over the
  command's multiplexed connection every second. It forwards listeners
  on `127.0.0.1`, `0.0.0.0`, `::1`, `::` and `*`, ports 1024 and up,
  except the guest's own (6080, 6081, 5900: the desktop, which `repose
  open --desktop` forwards). A listener only on another address (a
  Docker bridge, the guest's own IP) is not forwarded, and nor is a port
  bound only inside a Docker network.
- A forward is `ssh -O forward -L <laptop>:127.0.0.1:<port>` on the
  existing ControlMaster: no new connection, no process per port. It is
  cancelled (`ssh -O cancel`) within two seconds of the listener closing,
  and every forward is cancelled when the attach ends.
- The laptop port is the guest's port when it is free; otherwise the next
  free one, and the message says so: `⇄ localhost:3001 → :3000 (3000 is
  taken on your laptop)`. Portless's proxy port 1355 is never remapped
  silently, since the URLs portless prints name it: `1355 is taken on
  your laptop (portless?); izma's portless is on localhost:1356`. A port
  no laptop port could be found for is tried again after 30 seconds.
- Output is only ever inside tmux: `display-message` per new forward, and
  the session's `status-right` set to `⇄ <ports> │ <the global
  status-right>` while any attached CLI forwards. Each CLI keeps its
  ports in `~/.repose/forwards/<id>` in the guest (re-stamped every 20 s,
  ignored after 60 s), so two laptops attached to one project each forward
  on their own laptop and the bar shows the union; the session's own
  `status-right` is unset again when the last one leaves.
- `REPOSE_NO_FORWARD=1` turns it off, for a laptop whose ports must stay
  free. It is the only knob.

`repose open`:

- `repose open PORT [--local-port N] [--no-browser]` runs `ssh -N -L
  <local>:127.0.0.1:<port> <slug>.repose` using the CLI's SSH config, so
  anything the CLI can reach, a plain `ssh -L` can reach too, and opens
  the URL in the default browser unless `--no-browser`. One port per
  invocation; there is no multi-port, `--background`, `--list` or
  `--stop` form in the first release (that needs a forwards registry this
  workstream did not build).
- The local port defaults to the port number. If it is taken, the CLI
  picks a free one and forwards to that instead, with a message saying
  so, rather than failing.
- Forwards run in the foreground and die with the CLI (Ctrl-C, or the
  parent process exiting); nothing survives the CLI process to reattach
  to later.
- Anything bound on `0.0.0.0` or `127.0.0.1` in the guest is reachable this
  way. Nothing in the guest is reachable any other way; the guest has no
  inbound path except through the gateway.
- `repose open --desktop` starts the guest's desktop chain over SSH
  (`systemctl --user start repose-desktop`) and forwards 6080; Ctrl-C
  stops only the forward, not the desktop.

## Designed for later: preview URLs

`https://3000-todo-app.repose.herakraft.co` reaching port 3000 in the guest,
so a teammate or a phone can see a running dev server and a browser-driving
agent on the laptop can hit it.

Design (DECISIONS R2-6 chose to document it now):

- DNS: wildcard `*.repose.herakraft.co` to the edge. TLS: a wildcard
  certificate from Let's Encrypt via DNS-01, renewed on the edge.
- The gateway process on the edge gains an HTTPS listener. It parses
  `<port>-<slug>.repose.herakraft.co`, resolves `(slug, owner)` through the
  API (`GET /internal/route` extended to accept a slug without a handle and
  return the owner; slugs are unique per user, not globally, so the URL
  form must carry the handle: `3000-todo-app-heracraft.repose.herakraft.co`,
  with the handle as the last segment before the domain).
- Authentication: a session cookie issued by the dashboard after Logto
  login, scoped to `.repose.herakraft.co`. A request without it redirects
  to the dashboard login with a return URL. The cookie identifies the user;
  the gateway checks that user owns the project. A project can be marked
  `preview: public` by its owner to skip the check for read-only sharing;
  the default is owner-only.
- Proxying: HTTP/1.1 and WebSocket to `guest_ip:<port>` over WireGuard.
  Server-sent events and long polls pass through. Timeouts: 60 seconds idle.
  Response bodies are not buffered.
- Rate limits and abuse: a preview marked public is a way to serve content
  from a guest to the internet, so public previews count against egress and
  are capped at 100 requests per second per project, and the terms say what
  they may not be used for.
- The guest sees requests from the edge's WireGuard address with
  `X-Forwarded-For`, `X-Forwarded-Proto: https`, and `Host` rewritten to
  the preview hostname so frameworks' host checks pass. The guest base sets
  `VITE_ALLOWED_HOSTS` and the Next.js equivalent for `*.repose.herakraft.co`.
- The dashboard shows a project's open ports with a preview link each.

What is *not* in that design: custom domains per project, HTTP basic auth
as an alternative to the cookie, and non-HTTP protocols. Those wait for
demand.

## Depends on

Workstreams 07 (`open`), 04 (listening ports in signals),
06 (preview proxy, later), 11 (wildcard DNS and certificate, later), 05
(route by slug and owner, `preview` flag, later), 08 (preview links, later).

## Deferred

Preview URLs as designed above. Custom domains. Tunnelling arbitrary TCP.
