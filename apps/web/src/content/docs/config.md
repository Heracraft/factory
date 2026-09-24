---
title: Packages and configuration
description: Adding tools, languages and services to a project's machine, from a menu or with Nix.
section: Using repose
order: 18
---

Every machine starts from the same base (see [The machine](/docs/machine) for what's in it). There are three ways to add to it. Pick by how long you want the addition to last.

| Way                                                               | Takes effect                          | Survives a stop | Survives a rebuild or a move to another server |
| ----------------------------------------------------------------- | ------------------------------------- | --------------- | ---------------------------------------------- |
| Install it on the machine (`npm i -g`, `nix profile add`)         | At once                               | Yes             | No                                             |
| Add it to the project's configuration (menu, `repose config add`) | After a build, usually under a minute | Yes             | Yes                                            |
| Write the configuration in Nix yourself                           | After a build                         | Yes             | Yes                                            |

## Install on the machine

The machine is a normal Linux box, and installs you make there stay on its disk:

```
npm i -g tsx
uv tool install httpie
go install github.com/air-verse/air@latest
nix profile add nixpkgs#ffmpeg
```

`npm i -g` installs into `~/.npm-global`, which is on your `PATH`. `nix profile add` takes any package from nixpkgs; find names at [search.nixos.org](https://search.nixos.org/packages).

Type a command the machine doesn't have and it tells you which package has it and both ways to add it:

```
$ air
air is not installed. It is in the nixpkgs package air:
  now, in this guest:              nix profile add nixpkgs#air
  from your laptop, kept for good: repose config add air
```

This is the quickest path, and it's fine for most things. It isn't recorded anywhere outside the machine, though. If the project is ever rebuilt from its configuration, or restored onto a new server after a failure, installs made this way are kept only if they're in the snapshot the restore came from.

## The menu

The project's configuration page in the dashboard has a **Menu** tab listing languages, databases and tools. Tick what you want, choose a version where there's a choice, and **Apply**. The build log streams underneath.

| Group     | Entries                                                                                                                                           |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| Runtimes  | Bun, Deno, a second Node.js (20 or 22), a second Python (3.11 or 3.13), Go tools (gopls, golangci-lint, delve), Zig, Elixir, Ruby, Java, .NET SDK |
| Databases | PostgreSQL, Redis, MySQL (MariaDB), Memcached, RabbitMQ, Meilisearch, NATS                                                                        |
| Tools     | AWS CLI, OpenTofu, Kubernetes tools, Shell extras                                                                                                 |
| Deploy    | Wrangler (Cloudflare), Supabase CLI, flyctl (Fly.io), Vercel CLI, portless                                                                        |

Databases and message queues run as services on the machine, listening on localhost only. PostgreSQL comes with a `dev` role (superuser) and a `dev` database, and local connections need no password, so `psql` and `postgres://localhost/dev` work with no setup.

## `repose config add`

> Not in a release yet. v0.1.9 and older don't have `config add` or `config remove`; use the dashboard's menu.

The same thing from your terminal. A name from the menu adds that entry, services included. Any other name is looked up in nixpkgs:

```
$ repose config add bun postgresql gcc air
Added bun, postgresql, gcc and air to todo-app. Building revision 4f1c2a9e ...
Applied revision 4f1c2a9e.
```

Nested attribute names work too: `repose config add python312Packages.black nodePackages.typescript`.

A name nixpkgs doesn't have fails the build and changes nothing:

```
$ repose config add gcc-typo
Added gcc-typo to todo-app. Building revision 7d03b1c5 ...
config error: nixpkgs has no package "gcc-typo"; search https://search.nixos.org/packages
Nothing changed in todo-app; the previous revision is still active.
```

Remove with `repose config remove air` (or `rm`). Packages added this way appear under **Extra packages** on the dashboard's Menu tab.

## Writing Nix yourself

Under the menu is a Nix file, the project's _fragment_. It's a [home-manager](https://nix-community.github.io/home-manager/) module applied to the user `dev` on top of the base. See it with:

```
repose config show
```

Edit it in your `$EDITOR`, and apply it when you save and quit:

```
repose config edit
```

Or keep it in a file and apply that:

```
repose config apply ./repose.nix
```

(`repose config apply` with no path reads `./repose.nix`.) The dashboard's **Nix** tab has an editor too.

Writing the fragment yourself turns the menu off for the project, because the menu can't read arbitrary Nix. `repose config add` then refuses and tells you to edit the fragment instead. Applying from the dashboard's Menu tab later replaces your fragment with the menu's.

### An example

```nix
{ config, pkgs, lib, ... }:
{
  home.packages = with pkgs; [
    deno
    shellcheck
    hyperfine
  ];

  programs.git = {
    enable = true;
    extraConfig = {
      pull.rebase = true;
      init.defaultBranch = "main";
    };
  };

  # Written to ~/.config/htop/htoprc on every apply.
  xdg.configFile."htop/htoprc".text = ''
    tree_view=1
    hide_kernel_threads=1
  '';

  home.sessionVariables = {
    DENO_NO_UPDATE_CHECK = "1";
  };
}
```

### What a fragment can do

- Add packages from nixpkgs with `home.packages`.
- Configure `programs.*` and user `services.*` modules from home-manager.
- Write dotfiles with `home.file` and `xdg.configFile`.
- Set `home.sessionVariables` and `home.sessionPath`.
- Add overlays with `repose.overlays = [ (final: prev: { ... }) ]`. (home-manager's own `nixpkgs.overlays` is refused, because it would be ignored here.)
- Download sources with `pkgs.fetchurl`, `pkgs.fetchFromGitHub` and similar, always with a hash.
- Build things with any nixpkgs builder (`writeShellScriptBin`, `buildGoModule`, `buildNpmPackage`, ...).
- Turn on one of the database services through `repose.system`, which is what the menu writes:

```nix
{ pkgs, lib, ... }:
{
  repose.system = [
    {
      services.redis.servers.dev = {
        enable = true;
        port = 6379;
        bind = "127.0.0.1";
      };
    }
  ];
}
```

The services allowed there are `services.postgresql`, `services.redis`, `services.mysql`, `services.memcached`, `services.rabbitmq`, `services.meilisearch` and `services.nats`.

Unfree packages are refused except for this list: `claude-code`, `codex`, `gemini-cli`, `vscode`, `cursor`, `terraform`, `ngrok` and `google-chrome`.

### What a fragment can't do

- Set NixOS system options (`networking`, `users`, `boot`, `services.openssh` and anything not in the list above).
- Download at evaluation time: `builtins.fetchurl`, `fetchTarball`, `fetchGit` and `fetchTree` are refused. Use the `pkgs.fetch*` functions with a hash.
- Read files outside itself, or `import <nixpkgs>`. Use the `pkgs` argument.
- Import from a derivation.
- Choose its own nixpkgs. The base pins nixpkgs and home-manager, and every project on the same base uses the same versions.

### Limits

Evaluation gets 60 seconds. The build gets 30 minutes, 8 cores and 16 GB of memory. The finished configuration can add at most 20 GB to the base.

## Builds

Every change to the configuration (from the menu, `config add`, or your own fragment) is saved as a revision with an id like `4f1c2a9e` and built on the project's server. The CLI streams the build log. If the build fails, nothing changes: the machine keeps the previous revision, and the failed one is kept with its error so you can read it. A Nix error shows the line of your fragment it came from.

A successful build is switched into the running machine without a restart. Your tmux session and agents keep running, and new shells see the new packages.

Some changes (a different kernel, for instance) need a restart. The CLI says so:

```
This change needs a reboot; run `repose stop && repose start` when the agent is idle.
```

On a stopped machine, the new revision takes effect at the next start.

```
repose config show --revisions
```

lists revisions with their status and any error. To go back to an earlier one, use **Re-apply** next to it in the dashboard's Revisions list.

## Base updates

The platform releases a new base about once a week, sooner for security fixes, with updated agents, tools and kernel. Each project's configuration is rebuilt on the new base and switched in place. If your configuration doesn't build on the new base, the project stays where it was and you get a `base update failed` notification.

To stay on the current base, tick **Hold base updates** on the project's configuration page in the dashboard. Untick it to catch up.
