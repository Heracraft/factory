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
      username = "azureuser";
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
          # The nix node's default global prefix is its own store path, which
          # is read-only, so `npm i -g` fails with EACCES without this.
          sessionVariables.NPM_CONFIG_PREFIX = "${homeDirectory}/.npm-global";
          sessionPath = [
            "${homeDirectory}/.nix-profile/bin"
            "${homeDirectory}/.local/share/pnpm"
            "${homeDirectory}/.npm-global/bin"
            "${homeDirectory}/.cargo/bin"
          ];
        };

        # Makes `home-manager` itself available on the VM after the first switch.
        programs.home-manager.enable = true;

        # Required. Every integration below writes into programs.bash.initExtra,
        # and that whole block is gated on this being true. It also writes a
        # ~/.bash_profile that sources ~/.bashrc, which is what gets the hooks
        # into an SSH login shell.
        programs.bash = {
          enable = true;

          # Home Manager owns ~/.profile and its version only sources
          # hm-session-vars.sh, which never puts nix itself on PATH. The nix
          # installer's line lived in the ~/.profile we overwrote, so without
          # this a fresh login has no nix, no claude, nothing.
          profileExtra = ''
            if [ -e /nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh ]; then
              . /nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh
            elif [ -e "$HOME/.nix-profile/etc/profile.d/nix.sh" ]; then
              . "$HOME/.nix-profile/etc/profile.d/nix.sh"
            fi
          '';

          shellAliases = {
            # eza, carried over from the local fish config.
            ls = "eza -al --group-directories-first --no-permissions --no-user";
            lsz = "eza -al --total-size --group-directories-first";
            la = "eza -a --group-directories-first";
            ll = "eza -l --group-directories-first";
            lt = "eza -aT --group-directories-first";
            "l." = "eza -ald --group-directories-first .*";

            ".." = "cd ..";
            "..." = "cd ../..";
            "...." = "cd ../../..";
            "....." = "cd ../../../..";
            "......" = "cd ../../../../..";
          };
        };

        # Ships eza and a default set of ls aliases. The explicit aliases above
        # are plain assignments and the module's are mkDefault, so ours win.
        programs.eza.enable = true;

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
