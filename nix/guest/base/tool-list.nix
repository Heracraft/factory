# The tool list, as a function of pkgs so nix/flake.nix's devShell and the
# guest base share it. Moved from packages/core/flake.nix (curl, wget, jq,
# ripgrep, node 24, pnpm, python 3.12, uv, go, rustup, just) plus the
# additions docs/workstreams/02-guest-base.md names.
pkgs: with pkgs; [
  curl wget jq ripgrep fd bat fzf tree unzip zstd htop
  git gh just
  nodejs_24 pnpm python312 uv go rustup
  tmux openssh
  eza zoxide starship direnv nix-direnv
  neovim
  docker-compose
]
