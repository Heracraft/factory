# Ports and previews

A dev server running in the guest reaches the user's browser through an SSH
port forward the CLI manages. Public preview URLs per project are designed
here and built later, so the gateway is written with that path in mind.

## What the user sees, first release

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
