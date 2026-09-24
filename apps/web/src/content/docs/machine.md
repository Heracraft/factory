---
title: The machine
description: What's installed on a project's machine, how it's laid out, and its limits.
section: Using repose
order: 19
---

Each project runs in its own NixOS virtual machine (a Cloud Hypervisor microVM) with its own kernel, disk, memory and network address. The only thing it reads from the server it runs on is the Nix store holding the base and your configuration, mounted read-only.

## You and the layout

You log in as `dev`. `dev` has passwordless `sudo` and is in the `docker` group.

| Path                   | What                                                                                            |
| ---------------------- | ----------------------------------------------------------------------------------------------- |
| `/home/dev/<project>`  | Your checkout. tmux windows open here.                                                          |
| `/home/dev`            | Your home directory, on the project's disk. Everything here survives stops and is in snapshots. |
| `/run/repose/secrets/` | Your [named secrets](/docs/secrets), in memory only.                                            |
| `~/.repose/`           | repose's own bookkeeping on the machine. Leave it alone.                                        |

The environment variable `REPOSE=1` is set in every shell, and `REPOSE_PROJECT` holds the project's name. Scripts can use them to tell they're on a repose machine.

## What's installed

**Agents:** Claude Code, Codex CLI, opencode, Gemini CLI and pi. See [Agents](/docs/agents).

**Languages and package managers:** Node.js 24 with npm and pnpm, Python 3.12 with uv, Go, and rustup (run `rustup default stable` once to get a Rust toolchain).

**C and C++:** gcc, g++, make, cmake, pkg-config and binutils, for cgo, node-gyp, Python extensions and Rust's linker.

**Containers:** Docker with `docker compose`.

**Browser:** Chromium, Playwright's browsers, and the two browser MCP servers. See [Browser](/docs/browser).

**Command-line tools:** git, gh, just, curl, wget, jq, ripgrep, fd, bat, fzf, eza, tree, htop, tmux, neovim (the default `$EDITOR`), direnv with nix-direnv, starship, zoxide, sqlite, `psql` and the PostgreSQL client tools (no server; add one from the [menu](/docs/config#the-menu)), openssl, gnupg, dig, lsof, file, zip, unzip and zstd.

Need something else? [Packages and configuration](/docs/config) covers every way to add it.

### Programs built for other Linux systems

Prebuilt binaries that expect a regular Linux layout (Prisma's engines, esbuild and Biome from npm, anything a `curl | sh` installer drops) run as they would on Ubuntu. The machine provides the standard dynamic loader and common libraries for them.

### Projects with a `flake.nix`

direnv and nix-direnv are set up. Put `use flake` in the repository's `.envrc`, run `direnv allow` once on the machine, and the flake's dev shell loads whenever you `cd` into the checkout.

## Network

The machine can reach the internet. Nothing on the internet can reach the machine: the only way in is SSH through repose's gateway, with your certificate. See [Ports and localhost](/docs/ports) for reaching your own servers on it.

Outbound traffic is limited to 200 Mbit/s per machine. The first 500 GB each month is included, then it's billed (see [Pricing](/docs/billing)).

npm, pnpm and yarn downloads and Docker Hub pulls go through a cache on the server first, which makes repeated installs faster. A project with its own `.npmrc` registry, or an npm token for registry.npmjs.org in `~/.npmrc`, goes direct instead. A Docker login or a private registry also goes direct.

## Memory

| Size    | Memory | Browser limit |
| ------- | ------ | ------------- |
| `small` | 4 GB   | 1.5 GB        |
| `large` | 8 GB   | 3 GB          |
| `xl`    | 16 GB  | 6 GB          |

A machine that runs out of memory has to kill something. The machine arranges for your agents and the tmux server to be the last candidates, so a runaway dev server or test process goes first. To see what was killed, run `sudo dmesg | grep -i killed` on the machine.

`repose status todo-app` lists the processes listening on ports, with their age and memory, which helps spot a dev server left running for three days.

## Disk

The disk is 20, 40 or 80 GB depending on size. Grow it from the project's page in the dashboard (**Resize…** under Disk). It can't shrink.

The Nix store on the machine is layered: the base and your configuration come read-only from the server, and anything you install on the machine goes into a writable layer on your disk.

## Time

The machine's clock follows your laptop's time zone. See [Run and attach](/docs/run-and-attach#time-zone).
