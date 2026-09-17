# Toolchain every guest has. This is the successor of packages/core/flake.nix;
# that flake stays until the first guest boots from here (docs/DESIGN.md §17).
{ pkgs, ... }:
{
  environment.systemPackages = with pkgs; [
    curl wget jq ripgrep git gh just
    nodejs_24 pnpm python312 uv go rustup
    tmux eza zoxide starship direnv nix-direnv
    neovim
  ];
}
