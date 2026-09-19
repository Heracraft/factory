# Toolchain every guest has. Successor of packages/core/flake.nix (deleted
# in this workstream); the dev shell in nix/flake.nix reuses `toolPackages`
# so the dev box and the guests carry the same tools.
{ pkgs, lib, ... }:
let
  toolPackages = import ./tool-list.nix pkgs;
in
{
  environment.systemPackages = toolPackages;

  programs.direnv = {
    enable = true;
    nix-direnv.enable = true;
  };
  programs.starship.enable = true;
  programs.zoxide.enable = true;
  programs.git.enable = true;
  programs.neovim = {
    enable = true;
    defaultEditor = false;
  };
  programs.bash.completion.enable = true;
  programs.bash.shellAliases = {
    ls = "eza -al --group-directories-first --no-permissions --no-user";
    la = "eza -a --group-directories-first";
    ll = "eza -l --group-directories-first";
    lt = "eza -aT --group-directories-first";
  };
}
