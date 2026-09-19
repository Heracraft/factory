{
  description = "repose: hosts, edge, guest base, agent overlay, dev shell";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    home-manager = {
      url = "github:nix-community/home-manager";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    microvm = {
      url = "github:microvm-nix/microvm.nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    disko = {
      url = "github:nix-community/disko";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, home-manager, microvm, disko }:
    let
      system = "x86_64-linux";
      overlay = import ./overlay/agents;
      pkgs = import nixpkgs {
        inherit system;
        overlays = [ overlay ];
        config.allowUnfreePredicate = pkg:
          builtins.elem (nixpkgs.lib.getName pkg) [ "claude-code" ];
      };

      # Workstream 03 adds `packages.hostd`; until it exists the host runs
      # the stub from nix/hosts/hostd-stub.nix.
      hostdPackage = self.packages.${system}.hostd or null;
      hostModules = [
        disko.nixosModules.disko
        ./hosts
        { repose.host.hostdPackage = nixpkgs.lib.mkIf (hostdPackage != null) hostdPackage; }
      ];
      mkHost = { hostName, provider ? "azure", modules ? [ ] }:
        nixpkgs.lib.nixosSystem {
          inherit system;
          specialArgs = { inherit self; };
          modules = hostModules ++ [ { repose.host = { inherit hostName provider; }; } ] ++ modules;
        };
    in {
      overlays.agents = overlay;

      # Host: nixos-anywhere target. docs/workstreams/01-host-nixos.md
      # `host` is the generic configuration (what `nix build
      # .#nixosConfigurations.host...` in the launch prompt refers to);
      # named hosts are instances of mkHost with their own hostName.
      nixosConfigurations.host = mkHost { hostName = "repose-host"; };
      nixosConfigurations.host-bench = mkHost { hostName = "host-bench"; };
      lib.mkHost = mkHost;

      packages.${system}.hostd-stub = pkgs.callPackage ./hosts/hostd-stub.nix { };

      # NixOS VM tests for the host configuration (nix/hosts/tests). They
      # need KVM on the builder: `system-features = kvm` in nix.conf.
      checks.${system} = import ./hosts/tests {
        inherit pkgs nixpkgs disko;
        hostModules = hostModules;
      };

      # Edge: gateway + WireGuard hub. docs/workstreams/06-gateway-edge.md
      nixosConfigurations.edge = nixpkgs.lib.nixosSystem {
        inherit system;
        specialArgs = { inherit self; };
        modules = [ disko.nixosModules.disko ./edge ];
      };

      # Guest base as a module, and a function hostd calls with a user
      # fragment to produce a runner. docs/workstreams/02-guest-base.md and
      # docs/workstreams/12-nix-config-pipeline.md
      nixosModules.guestBase = import ./guest/base;
      lib.mkGuest = import ./guest/microvm.nix {
        inherit nixpkgs home-manager microvm system overlay self;
      };

      devShells.${system} = {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go_1_26 gopls golangci-lint buf protoc-gen-go protoc-gen-go-grpc
            opentofu azure-cli just nixos-anywhere nixos-rebuild
            postgresql_16 sqlc wireguard-tools
          ];
        };

        # `nix develop ./nix#infra`. tfsec runs the policies in
        # infra/policy/tfsec; it is here rather than in the default shell
        # because only workstream 11 needs it.
        # docs/workstreams/11-infra-opentofu.md §7.
        infra = pkgs.mkShell {
          packages = with pkgs; [
            opentofu azure-cli tfsec just jq nixos-anywhere wireguard-tools
          ];
        };
      };

      formatter.${system} = pkgs.nixfmt;
    };
}
