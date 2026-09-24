---
title: The machine
description: What's installed, installing more, your laptop's tools, ports, the browser, memory and disk.
section: Using repose
order: 12
---

Each project gets its own virtual machine running NixOS, with its own kernel, disk, memory and Docker. You log in as `dev`, which has passwordless `sudo`. Your checkout is `/home/dev/<project>`, and everything under `/home/dev` survives a stop and is in snapshots.

| Size    | vCPU | Memory | Disk  |
| ------- | ---- | ------ | ----- |
| `small` | 2    | 4 GB   | 20 GB |
| `large` | 4    | 8 GB   | 40 GB |
| `xl`    | 8    | 16 GB  | 80 GB |

## What's installed

- **Agents:** Claude Code, Codex CLI, opencode, Gemini CLI and pi.
- **Languages:** Node.js 24 with npm and pnpm, Python 3.12 with uv, Go, and rustup (run `rustup default stable` once).
- **Build tools:** gcc, g++, make, cmake, pkg-config, so cgo, node-gyp, Python extensions and Rust crates like `openssl-sys` build.
- **Containers:** Docker with `docker compose`.
- **Browser:** Chromium and Playwright's browsers.
- **Everyday tools:** git, gh, tmux, just, curl, wget, jq, ripgrep, fd, bat, fzf, eza, zoxide, tree, htop, neovim, direnv, sqlite3, `psql`, `pg_dump` and `pg_restore` (no database server; [add one](/docs/config)), openssl, gnupg, dig, lsof, killall, file, zip, unzip and zstd.

The shell is bash with the starship prompt, and `ls` is `eza -al`. `dev` is in the `docker` group, so `docker` needs no `sudo`.

Programs downloaded for other Linux systems run as they would on Ubuntu: Prisma's engines, Playwright's own browsers, numpy and other Python wheels, esbuild, Biome, and binaries from `curl | sh` installers.

## Installing more

Install on the machine the way you would anywhere. The result stays on the machine's disk and is on your `PATH`:

```
npm i -g tsx
go install github.com/air-verse/air@latest
cargo install ripgrep-all
uv tool install httpie
nix profile add nixpkgs#ffmpeg
```

`pip install --user`, bun, deno, gem and composer installs are on `PATH` too, as are the usual directories of yarn, dotnet, ghcup, cabal, opam, luarocks, mix, nimble, juliaup, krew and volta. `nix profile add` takes any package from nixpkgs; search names at [search.nixos.org](https://search.nixos.org/packages).

Type a command the machine doesn't have and it tells you where to get it:

```
$ air
air is not installed. It is in the nixpkgs package air:
  now, in this guest:              nix profile add nixpkgs#air
  from your laptop, kept for good: repose config add air
```

Installs made on the machine are not part of the project's configuration. To have a package on every rebuild, add it with [`repose config add`](/docs/config).

## Your laptop's tools come along

`repose run` looks at the tools you installed globally on your laptop (with npm, pnpm, bun, `go install`, `cargo install`, uv or pipx) and at the commands your project's scripts call (`package.json`, `Makefile`, `justfile`, `Procfile`, `.air.toml`, compose files). Only names and versions are sent. The machine installs the ones it lacks in the background:

```
Installing 3 of your tools in the background: air, portless, typescript
```

Each comes from nixpkgs when nixpkgs has it, so its version can differ from your laptop's; otherwise your laptop's version is installed with its own package manager. Nothing waits for these. If a tool fails to install, the next `run` says so; the log is `~/.repose/tools-install.log` on the machine. A Node major version pinned in `.nvmrc`, `.node-version`, `.tool-versions`, `volta.node` or `engines.node` (the first found) is installed and made the default `node`.

To see the list without installing anything:

```
repose scan
```

## Projects with a flake.nix

direnv is set up. Put `use flake` in the repository's `.envrc`, run `direnv allow` once on the machine, and the flake's dev shell loads when you `cd` into the checkout.

## Ports

While you're attached with `repose run` or `repose attach`, every port a program on the machine listens on appears on your laptop's `localhost` within a second or so. Start `pnpm dev` on the machine and open `http://localhost:5173` on your laptop. tmux shows each new forward:

```
⇄ localhost:5173 → :5173
```

Because it's `localhost`, cookies and OAuth redirects behave as they do locally. If the port is taken on your laptop, the next free one (up to 20 higher) is used and the message says which.

Ports below 1024 aren't forwarded, nor are the machine's own 5353, 5355, 5900, 6080 and 6081, nor servers that listen only on another address such as a Docker network. A container port published with `-p 8080:80` is.

To forward one port without attaching:

```
repose open 3000
```

It opens your browser and runs until `Ctrl-C`. `--local-port 8080` picks the laptop port, `--no-browser` only prints the URL. `REPOSE_NO_FORWARD=1` turns the automatic forwarding off.

There are no public URLs for a project's ports. To show someone a running app, deploy it or use a tunnel such as `cloudflared`.

## Browser

Claude Code on the machine has two browser tools registered, `playwright` and `chrome-devtools`: navigate, fill forms, take screenshots, read the console and network. Both drive the same Chromium, which starts the first time an agent uses one of them and keeps its cookies and logins between runs. Ask for them in a prompt:

```
repose run "start the dev server, open the signup page with playwright and screenshot each step"
```

Playwright test suites run without `npx playwright install`.

To watch the browser or use it yourself (a captcha, a passkey), open the machine's desktop:

```
$ repose open --desktop
http://localhost:6080/vnc.html?autoconnect=1 (Ctrl-C stops the forward; the desktop keeps running)
VNC password: 5m2k8Q1p
```

Enter the password in the page that opens. You see the agent's browser as it works, in the same window the agent uses, so you can click and type in it: solve a captcha, log in, approve a passkey. Whatever you log into there, the agent's browser tools can use afterwards. If no agent has used the browser yet, opening the desktop starts it.

`repose open --desktop --stop` stops the viewer, and so does 30 minutes with nobody connected. The agent's browser keeps running while an agent uses it, and stops after 30 minutes with neither an agent nor you on it.

An agent session started before September 25, 2026 has a headless browser of its own that the desktop can't show; restart the agent to switch it over.

Playwright test suites (`pnpm exec playwright test` and the like) still run headless unless your config asks for headed browsers. While the agent's browser is up, new shells have `DISPLAY=:99`, so a headed browser you start yourself also appears on the desktop.

## Network

The machine can reach the internet. Nothing on the internet can reach the machine; the only way in is SSH through repose, with your certificate. Outbound traffic is limited to 200 Mbit/s. npm, pnpm, yarn v1 and Docker Hub downloads go through a cache on the server. For npm, repose adds two lines to `~/.npmrc`; delete them to go direct. An `~/.npmrc` that already names a registry or holds an npmjs token is left alone. [Limits](/docs/limits) has what's blocked.

## Memory and disk

When a machine runs out of memory, something is killed. Your agents and tmux are kept to the last, so a runaway test or dev server goes first. `sudo dmesg | grep -i killed` shows what went. Headless Chromium is stopped past 1.5, 3 or 6 GB depending on size.

Grow the disk with `repose resize 80G`, or from the project's page in the dashboard (**Resize…** under Disk, 20 to 320 GB). Disks can't shrink, and the larger disk is [billed](/docs/billing) from then on. A project's size is chosen when it's created and can't be changed afterwards yet.
