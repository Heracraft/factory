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

  # npm's default global prefix is the nodejs store path, which is read-only,
  # so `npm i -g` fails with EACCES. Point it at the dev home instead
  # (guest-conventions.md "Environment", DECISIONS I-6).
  environment.sessionVariables.NPM_CONFIG_PREFIX = "/home/dev/.npm-global";
  environment.extraInit = ''
    export PATH="/home/dev/.npm-global/bin:$PATH"
  '';
}
