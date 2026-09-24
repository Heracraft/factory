---
title: Config
description: Add packages and services to a project so they're there on every rebuild, from the CLI, the dashboard or Nix.
section: Using repose
order: 13
---

A project's configuration is the list of extra packages and services its machine is built with. Unlike an install you make on the machine, it's kept with the project, so it survives rebuilds, platform updates and a restore onto another server.

## Add a package

```
$ repose config add postgresql air nodejs_22
Added postgresql, air and nodejs_22 to todo-app. Building revision 4f1c2a9e ...
Applied revision 4f1c2a9e.
```

Any package from nixpkgs works; search names at [search.nixos.org](https://search.nixos.org/packages). Nested names work too, such as `python312Packages.black`. A few names are menu entries that set up more than a package: `postgresql`, `redis` and the other databases also start the service.

The build usually takes under a minute. It's switched into the running machine without a restart, so your agents keep running and new shells see the new packages. If the build fails, nothing changes:

```
$ repose config add gcc-typo
Added gcc-typo to todo-app. Building revision 7d03b1c5 ...
config error: nixpkgs has no package "gcc-typo"; search https://search.nixos.org/packages
Nothing changed in todo-app; the previous revision is still active.
```

Remove with:

```
repose config remove air
```

## The menu

The dashboard's project **Config** page has the same list as a menu: tick an entry, choose a version where there's a choice, **Apply**. Packages added with `repose config add` show under **Extra packages**, each with **Remove**. While a build runs, the page shows its log.

| Group     | Entries                                                                                           |
| --------- | ------------------------------------------------------------------------------------------------- |
| Runtimes  | Bun, Deno, Node.js (20 or 22), Python (3.11 or 3.13), Go tools, Zig, Elixir, Ruby, Java, .NET SDK |
| Databases | PostgreSQL, Redis, MySQL (MariaDB), Memcached, RabbitMQ, Meilisearch, NATS                        |
| Tools     | AWS CLI, OpenTofu, Kubernetes tools, Shell extras                                                 |
| Deploy    | Wrangler, Supabase CLI, flyctl, Vercel CLI, portless                                              |

Databases listen on localhost only. PostgreSQL has a `dev` superuser and a `dev` database with no password, so `psql` and `postgres://localhost/dev` work straight away.

## Write it in Nix

Under the menu is a Nix file, a [home-manager](https://nix-community.github.io/home-manager/) module for the user `dev`. Nix is the language NixOS machines are configured in. You only need it for things the menu can't express, such as dotfiles or environment variables.

```
repose config show            # print it
repose config edit            # edit in $EDITOR, apply on save
repose config apply ./repose.nix
```

Or use the **Nix** tab on the dashboard's Config page (**Edit as Nix** from the menu).

An example:

```nix
{ pkgs, ... }:
{
  home.packages = with pkgs; [ deno shellcheck hyperfine ];

  home.sessionVariables.DENO_NO_UPDATE_CHECK = "1";

  xdg.configFile."htop/htoprc".text = ''
    tree_view=1
  '';

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

Once you edit the Nix by hand, the menu and `repose config add` are off for that project, because they can't read arbitrary Nix. Applying from the menu later replaces your file.

What the file can't do: set NixOS system options other than the database services under `repose.system`, download without a hash, read files outside itself, or choose its own nixpkgs version. Don't put secrets in it; use [Secrets](/docs/secrets).

## Revisions and base updates

Every change is saved as a revision. `repose config show --revisions` lists them with any errors, and the dashboard can re-apply an earlier one. `repose logs --kind build` shows the last build's log.

The platform updates the base (agents, tools, kernel) about once a week. Each project is rebuilt on the new base and switched in place. If your configuration doesn't build on it, the project stays where it was and you get a notification. To hold a project on its current base, tick **Hold base updates** on its Config page.
