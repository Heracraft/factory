{
  description = "Basic dev tools";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    home-manager = {
      url = "github:nix-community/home-manager";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, home-manager }:
    let
      system = "x86_64-linux";

      # The VM's login user. Change both if the image uses ec2-user, debian, root, etc.
      username = "ubuntu";
      homeDirectory = "/home/${username}";

      pkgs = import nixpkgs {
        inherit system;
        config.allowUnfreePredicate = pkg: builtins.elem (nixpkgs.lib.getName pkg) [ "claude-code" ];
      };

      tools = with pkgs; [
        curl
        wget
        jq
        ripgrep

        claude-code
        opencode

        nodejs_24
        pnpm
        python312
        uv
        go
        rustup

        just
      ];

      homeModule = {
        home = {
          inherit username homeDirectory;
          stateVersion = "26.11";
          packages = tools;

          sessionVariables.PNPM_HOME = "${homeDirectory}/.local/share/pnpm";
          sessionPath = [
            "${homeDirectory}/.local/share/pnpm"
            "${homeDirectory}/.cargo/bin"
          ];
        };

        # Makes `home-manager` itself available on the VM after the first switch.
        programs.home-manager.enable = true;

        # Required. Every integration below writes into programs.bash.initExtra,
        # and that whole block is gated on this being true. It also writes a
        # ~/.bash_profile that sources ~/.bashrc, which is what gets the hooks
        # into an SSH login shell.
        programs.bash.enable = true;

        # These three are the ones that needed init lines.
        programs.zoxide.enable = true;
        programs.starship.enable = true;
        programs.direnv = {
          enable = true;
          nix-direnv.enable = true;
        };

        # Packaged and configurable. Empty for now, but this is where dotfile
        # config goes instead of a shell script.
        programs.git.enable = true;
        # programs.git.userName = "Heracraft";
        # programs.git.userEmail = "you@example.com";
        programs.gh.enable = true;
        programs.tmux.enable = true;
      };
    in {
      homeConfigurations.${username} = home-manager.lib.homeManagerConfiguration {
        inherit pkgs;
        modules = [ homeModule ];
      };

      # Kept so an existing `nix profile` install still upgrades during migration.
      packages.${system} = rec {
        dev-tools = pkgs.buildEnv {
          name = "dev-tools";
          paths = tools;
        };
        default = dev-tools;
      };
    };
}
