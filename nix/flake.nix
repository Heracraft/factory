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
      lib = nixpkgs.lib;
      overlay = import ./overlay/agents;
      pkgs = import nixpkgs {
        inherit system;
        overlays = [ overlay ];
        config.allowUnfreePredicate = pkg:
          builtins.elem (lib.getName pkg) [ "claude-code" "codex" "gemini-cli" ];
      };

      # The platform base version: the revision of this repository. The api's
      # base_versions row and /etc/repose/base-version carry the same string.
      baseVersion = self.shortRev or self.dirtyShortRev or "dirty";

      # Go binaries the guest runs, built from this repository. Only the Go
      # sources are part of the input so unrelated edits do not rebuild them.
      goSrc = lib.fileset.toSource {
        root = ../.;
        fileset = lib.fileset.unions ([
          ../go.mod
          ../cmd
          ../internal
        ] ++ lib.optional (builtins.pathExists ../go.sum) ../go.sum
          ++ lib.optional (builtins.pathExists ../proto) ../proto);
      };
      mkGo = name: pkgs.buildGoModule {
        pname = name;
        version = baseVersion;
        src = goSrc;
        subPackages = [ "cmd/${name}" ];
        vendorHash = null;
        ldflags = [ "-s" "-w" "-X main.version=${baseVersion}" ];
        meta.mainProgram = name;
      };
      guestd = mkGo "guestd";
      # Workstream 04 ships cmd/repose-hook; until then the overlay's shell
      # implementation of the same contract is used.
      reposeHook =
        if builtins.pathExists ../cmd/repose-hook/main.go
        then mkGo "repose-hook"
        else pkgs.repose-hook-shim;

      mkGuestRunner = import ./guest/microvm.nix {
        inherit nixpkgs home-manager microvm system overlay self;
      };

      guestTests = import ./guest/tests {
        inherit pkgs lib baseVersion;
        guestBase = self.nixosModules.guestBase;
        inherit guestd reposeHook;
      };
    in {
      overlays.agents = overlay;

      # Host: nixos-anywhere target. docs/workstreams/01-host-nixos.md
      nixosConfigurations.host = lib.nixosSystem {
        inherit system;
        specialArgs = { inherit self; };
        modules = [ disko.nixosModules.disko ./hosts ];
      };

      # Edge: gateway + WireGuard hub. docs/workstreams/06-gateway-edge.md
      nixosConfigurations.edge = lib.nixosSystem {
        inherit system;
        specialArgs = { inherit self; };
        modules = [ disko.nixosModules.disko ./edge ];
      };

      # Guest base as a module, and the function hostd's build step calls with
      # a user fragment to produce a runner. docs/workstreams/02-guest-base.md
      # and docs/workstreams/12-nix-config-pipeline.md
      nixosModules.guestBase = {
        imports = [ ./guest/base ];
        repose.baseVersion = lib.mkDefault baseVersion;
        repose.guestd.package = lib.mkDefault guestd;
        repose.hookPackage = lib.mkDefault reposeHook;
      };

      lib = {
        inherit mkGuestRunner;
        # Name used by the scaffold; same function.
        mkGuest = mkGuestRunner;
        inherit baseVersion;
      };

      packages.${system} = {
        inherit guestd;
        repose-hook = reposeHook;
        # A runner with an empty fragment: what `nix build .#guest-runner`
        # produces and what the local Cloud Hypervisor boot test uses.
        guest-runner = mkGuestRunner {
          inherit guestd;
          hook = reposeHook;
          baseVersion = baseVersion;
          class = "large";
        };
        guest-system = self.packages.${system}.guest-runner.toplevel;
        claude-code = pkgs.reposeAgents.claude-code;
        opencode = pkgs.reposeAgents.opencode;
        codex = pkgs.reposeAgents.codex;
        gemini-cli = pkgs.reposeAgents.gemini-cli;
        pi-coding-agent = pkgs.reposeAgents.pi-coding-agent;
        playwright-mcp = pkgs.reposeMcp.playwright-mcp;
        chrome-devtools-mcp = pkgs.reposeMcp.chrome-devtools-mcp;
        default = self.packages.${system}.guest-runner;
      };

      checks.${system} = guestTests // {
        # The base closure must stay under 6 GB (02 §7): every host's store
        # grows by it, and every first boot registers its metadata.
        guest-closure-size = pkgs.runCommand "guest-closure-size" {
          closureInfo = pkgs.closureInfo { rootPaths = [ self.packages.${system}.guest-system ]; };
        } ''
          size=$(cut -f2 $closureInfo/total-nar-size 2>/dev/null || true)
          [ -n "$size" ] || size=$(cat $closureInfo/total-nar-size)
          limit=$((6 * 1024 * 1024 * 1024))
          echo "guest system closure: $size bytes (limit $limit)"
          if [ "$size" -gt "$limit" ]; then
            echo "closure over 6 GB; largest paths:" >&2
            exit 1
          fi
          echo "$size" > $out
        '';
        guest-runner-builds = self.packages.${system}.guest-runner;
      };

      devShells.${system}.default = pkgs.mkShell {
        packages = (import ./guest/base/tool-list.nix pkgs) ++ (with pkgs; [
          reposeAgents.claude-code reposeAgents.opencode
          go_1_26 gopls golangci-lint buf protoc-gen-go protoc-gen-go-grpc
          opentofu azure-cli nixos-anywhere nixos-rebuild
          postgresql_16 sqlc wireguard-tools
        ]);
      };

      formatter.${system} = pkgs.nixfmt;
    };
}
