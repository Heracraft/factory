# Ports and previews

A dev server running in the guest reaches the user's browser through an SSH
port forward the CLI manages. Public preview URLs per project are designed
here and built later, so the gateway is written with that path in mind.

## What the user sees, first release

```
$ factory open 3000
Forwarding http://localhost:3000 -> todo-app:3000. Ctrl-C to stop.
```

```
$ factory open 3000 5173 --background
Forwarding 3000, 5173 in the background (pid 48213). `factory open --stop` ends it.
```

```
$ factory open --desktop
```

(browser.md covers the desktop.)

## Behaviour that must hold

- `factory open <port>...` runs `ssh -N -L <port>:127.0.0.1:<port>
  <slug>.factory` using the CLI's SSH config, so anything the CLI can reach,
  a plain `ssh -L` can reach too.
- The local port defaults to the same number. `factory open 3000:8080` maps
  a local 3000 to the guest's 8080. A busy local port is reported with the
  process using it when that can be determined.
- Forwards die with the CLI unless `--background`, which detaches and
  records the pid in `~/.config/factory/forwards.json`; `factory open
  --list` and `--stop` manage them.
- Anything bound on `0.0.0.0` or `127.0.0.1` in the guest is reachable this
  way. Nothing in the guest is reachable any other way; the guest has no
  inbound path except through the gateway.
- Forwards survive a certificate refresh because SSH keeps the established
  connection; a new forward after expiry triggers a refresh first.
- `factory status` shows the ports the guest is listening on (from guestd,
  via `ss -ltn`), so the user knows which number to open.

## Designed for later: preview URLs

`https://3000-todo-app.factory.herakraft.co` reaching port 3000 in the guest,
so a teammate or a phone can see a running dev server and a browser-driving
agent on the laptop can hit it.

Design (DECISIONS R2-6 chose to document it now):

- DNS: wildcard `*.factory.herakraft.co` to the edge. TLS: a wildcard
  certificate from Let's Encrypt via DNS-01, renewed on the edge.
- The gateway process on the edge gains an HTTPS listener. It parses
  `<port>-<slug>.factory.herakraft.co`, resolves `(slug, owner)` through the
  API (`GET /internal/route` extended to accept a slug without a handle and
  return the owner; slugs are unique per user, not globally, so the URL
  form must carry the handle: `3000-todo-app-heracraft.factory.herakraft.co`,
  with the handle as the last segment before the domain).
- Authentication: a session cookie issued by the dashboard after Logto
  login, scoped to `.factory.herakraft.co`. A request without it redirects
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
  `VITE_ALLOWED_HOSTS` and the Next.js equivalent for `*.factory.herakraft.co`.
- The dashboard shows a project's open ports with a preview link each.

What is *not* in that design: custom domains per project, HTTP basic auth
as an alternative to the cookie, and non-HTTP protocols. Those wait for
demand.

## Depends on

Workstreams 07 (`open`, forwards file), 04 (listening ports in signals),
06 (preview proxy, later), 11 (wildcard DNS and certificate, later), 05
(route by slug and owner, `preview` flag, later), 08 (preview links, later).

## Deferred

Preview URLs as designed above. Custom domains. Tunnelling arbitrary TCP.
