{
  description = "Basic dev tools";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs {
        inherit system;
        config.allowUnfreePredicate = pkg: builtins.elem (nixpkgs.lib.getName pkg) [ "claude-code" ];
      };
      dev-tools = pkgs.buildEnv {
        name = "dev-tools";
        paths = with pkgs; [
          git
          gh
          tmux
          zsh
          curl
          wget
          jq
          ripgrep
          fd
          fzf
          zoxide
          starship

          claude-code
          opencode

          nodejs_24
          pnpm
          python312
          uv
          go
          rustup

          just
          direnv
        ];
      };
    in {
      packages.${system} = {
        inherit dev-tools;
        default = dev-tools;
      };

    };
}