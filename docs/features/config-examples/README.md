# Example fragments

Each file is a complete fragment: paste it into `repose config edit`, or
`repose config apply ./file.nix`. `nix flake check ./nix` composes every
`*.nix` here on the platform base (`checks.fragment-examples`), so an
example that stops building fails CI. The contract they follow is in
[../config.md](../config.md) under "Writing a fragment".

| File | Shows |
|---|---|
| `packages-and-dotfiles.nix` | packages from nixpkgs, a dotfile, session variables, a home-manager program module |
| `overlay-and-custom-package.nix` | `repose.overlays`, a fixed-output fetch with a hash, a script built with `writeShellScriptBin` |
| `menu-postgres-and-bun.nix` | what the menu generates: a runtime package plus a catalog service through `repose.system` |
