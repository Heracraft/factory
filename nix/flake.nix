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
    in {
      overlays.agents = overlay;

      # Host: nixos-anywhere target. docs/workstreams/01-host-nixos.md
      nixosConfigurations.host = nixpkgs.lib.nixosSystem {
        inherit system;
        specialArgs = { inherit self; };
        modules = [ disko.nixosModules.disko ./hosts ];
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

      # hostd and hostdev (workstream 03). The Go module is the repository
      # root, one level above this flake, so build with the repo as the
      # flake source: `nix build 'git+file://.?dir=nix#hostd'` or
      # `nix build '.?dir=nix#hostd'` from the repository root. From
      # `./nix` alone the parent is not in the source and evaluation fails
      # with a clear message instead of a build error.
      packages.${system} =
        let
          src = ../.;
          goCommon = {
            version = "0.1.0";
            inherit src;
            vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
            env.CGO_ENABLED = 0;
            ldflags = [ "-s" "-w" ];
            meta.description = "repose host daemon (docs/workstreams/03-hostd.md)";
          };
          have = builtins.pathExists (src + "/go.mod");
          mk = name: if have then pkgs.buildGoModule (goCommon // {
            pname = name;
            subPackages = [ "cmd/${name}" ];
            ldflags = goCommon.ldflags ++ [ "-X main.version=${goCommon.version}" ];
          }) else throw "packages.${name}: build from the repository root with `nix build '.?dir=nix#${name}'` so go.mod is in the flake source";
        in {
          hostd = mk "hostd";
          hostdev = mk "hostdev";
        };

      devShells.${system}.default = pkgs.mkShell {
        packages = with pkgs; [
          go_1_26 gopls golangci-lint buf protoc-gen-go protoc-gen-go-grpc
          opentofu azure-cli just nixos-anywhere nixos-rebuild
          postgresql_16 sqlc wireguard-tools
        ];
      };

      formatter.${system} = pkgs.nixfmt;
    };
}
