{
  description = "Basic dev tools";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };
    in {
      packages.${system}.agent-dev-tools = pkgs.buildEnv {
        name = "agent-dev-tools";

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

          nodejs_22
          pnpm
          python312
          uv
          go
          rustup

          just
          direnv
        ];
      };

      packages.${system}.default = self.packages.${system}.agent-dev-tools;
    };
}