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
    # The user fragment for `guestSystem`. The placeholder keeps the lock
    # file valid; hostd overrides it per Build with
    # `--override-input fragment path:/var/lib/repose/builds/<rev>`
    # (docs/interfaces/nix-build-contract.md, DECISIONS I-28).
    fragment = {
      url = "path:./guest/fragment-placeholder";
      flake = false;
    };
  };

  outputs = { self, nixpkgs, home-manager, microvm, disko, fragment }:
    let
      system = "x86_64-linux";
      lib = nixpkgs.lib;
      overlay = import ./overlay/agents;
      pkgs = import nixpkgs {
        inherit system;
        overlays = [ overlay ];
        config.allowUnfreePredicate = pkg: builtins.elem (lib.getName pkg) (import ./guest/unfree-allowlist.nix);
      };

      # The platform base version: the revision of this repository. The api's
      # base_versions row and /etc/repose/base-version carry the same string.
      baseVersion = self.shortRev or self.dirtyShortRev or "dirty";

      # Every Go binary in the repository (guestd, repose-hook, hostd,
      # hostdev), built from the repository root one level above this flake.
      # From `./nix` alone the parent is not in the flake source, so build with
      # `nix build '.?dir=nix#hostd'` from the repository root.
      goPkgs = import ./packages.nix { inherit pkgs lib; version = baseVersion; };
      guestd = goPkgs.guestd;
      reposeHook = goPkgs.repose-hook;

      mkGuestRunner = import ./guest/microvm.nix {
        inherit nixpkgs home-manager microvm system overlay self;
      };
      composeGuest = import ./guest/compose.nix { inherit mkGuestRunner; };
      # The fragment pipeline's checks: every example fragment composes and
      # builds, and the contract's refusals refuse (nix/guest/checks.nix).
      fragmentChecks = import ./guest/checks.nix {
        inherit pkgs lib composeGuest guestd baseVersion;
        hook = reposeHook;
        examplesDir = ../docs/features/config-examples;
      };

      guestTests = import ./guest/tests {
        inherit pkgs lib baseVersion;
        guestBase = self.nixosModules.guestBase;
        inherit guestd reposeHook;
      };

      hostModules = [
        disko.nixosModules.disko
        ./hosts
        { repose.host.hostdPackage = goPkgs.hostd; }
      ];
      mkHost = { hostName, provider ? "azure", modules ? [ ] }:
        lib.nixosSystem {
          inherit system;
          specialArgs = { inherit self; };
          modules = hostModules ++ [ { repose.host = { inherit hostName provider; }; } ] ++ modules;
        };
      hostChecks = import ./hosts/tests {
        inherit pkgs nixpkgs disko hostModules;
      };
    in {
      overlays.agents = overlay;

      # Hosts: nixos-anywhere targets. docs/workstreams/01-host-nixos.md.
      # `host` is the generic configuration; named hosts are instances of
      # lib.mkHost with their own hostName.
      nixosConfigurations.host = mkHost { hostName = "repose-host"; };
      nixosConfigurations.host-bench = mkHost { hostName = "host-bench"; };
      # Production hosts by name (infra `host_flake_attrs`); each names what
      # differs from `host`: today the api address, its CA and the Blob
      # account (DECISIONS I-40).
      nixosConfigurations.host-01 = mkHost { hostName = "host-01"; modules = [ ./hosts/host-01.nix ]; };

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
        inherit mkGuestRunner mkHost baseVersion composeGuest;
        # Name used by the scaffold; same function.
        mkGuest = mkGuestRunner;
      };

      # What hostd evaluates for a Build (docs/interfaces/nix-build-contract.md):
      # the base plus the fragment at "${fragment}/fragment.nix" applied to
      # dev. `config.system.build.toplevel` is the system closure; the class
      # is not baked in (DECISIONS I-34, I-42).
      guestSystem = (composeGuest {
        fragmentPath = "${fragment}/fragment.nix";
        inherit guestd baseVersion;
        hook = reposeHook;
      }).guestSystem;

      packages.${system} = {
        inherit (goPkgs) guestd repose-hook hostd hostdev api repose-admin;
        # Workstream 01's stand-in, kept for the host VM tests.
        hostd-stub = pkgs.callPackage ./hosts/hostd-stub.nix { };
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

      # NixOS VM tests for the host (nix/hosts/tests) and the guest
      # (nix/guest/tests). They need KVM on the builder: `system-features =
      # kvm` in nix.conf.
      checks.${system} = hostChecks // guestTests // fragmentChecks // {
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
        # docs/workstreams/04-guestd.md §7: the real binary exercised inside a
        # real guest.
        guestd = import ./guest/tests/guestd.nix {
          inherit pkgs;
          guestdPackage = guestd;
        };
      };

      devShells.${system} = {
        default = pkgs.mkShell {
          packages = (import ./guest/base/tool-list.nix pkgs) ++ (with pkgs; [
            reposeAgents.claude-code reposeAgents.opencode
            go_1_26 gopls golangci-lint buf protoc-gen-go protoc-gen-go-grpc
            opentofu azure-cli just nixos-anywhere nixos-rebuild
            postgresql_16 sqlc wireguard-tools
          ]);
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
