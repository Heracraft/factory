# The five agents from the platform overlay, wrapped, plus repose-hook and
# the platform hook and MCP configuration the wrappers merge into the user's
# own files on first run (guest-conventions.md "Agent wrappers").
{ config, lib, pkgs, ... }:
let
  claudeSettings = {
    hooks = {
      Notification = [
        { matcher = ""; hooks = [ { type = "command"; command = "repose-hook"; } ]; }
      ];
      Stop = [
        { matcher = ""; hooks = [ { type = "command"; command = "repose-hook"; } ]; }
      ];
    };
  };
in
{
  options.repose.applyOverlay = lib.mkOption {
    type = lib.types.bool;
    default = true;
    description = ''
      Whether this module adds the agents overlay and the unfree allowlist to
      nixpkgs itself. Off when the evaluation supplies a ready pkgs (the
      NixOS test driver does), which must then carry the overlay already.
    '';
  };

  options.repose.hookPackage = lib.mkOption {
    type = lib.types.package;
    default = pkgs.repose-hook-shim;
    defaultText = "pkgs.repose-hook-shim";
    description = ''
      The repose-hook binary agents call from their hooks. Defaults to the
      overlay's shell implementation; nix/flake.nix sets the Go binary from
      cmd/repose-hook (workstream 04) once it exists.
    '';
  };

  config = lib.mkMerge [ (lib.mkIf config.repose.applyOverlay {
    # The base brings its own overlay so nixosModules.guestBase is
    # self-contained; nix/guest/microvm.nix adds the user's overlays after.
    nixpkgs.overlays = [ (import ../../overlay/agents) ];
    nixpkgs.config.allowUnfreePredicate = pkg: builtins.elem (lib.getName pkg) (import ../unfree-allowlist.nix);
  }) {

    environment.systemPackages = (builtins.attrValues pkgs.reposeAgents) ++ [
      config.repose.hookPackage
      pkgs.repose-agent-setup
    ];

    environment.etc."repose/claude-settings.json".text = builtins.toJSON claudeSettings;

    # Every agent that has a hook system is registered here; the rest use
    # guestd's pane-idle heuristic (docs/features/agents.md).
    environment.etc."repose/agents.json".text = builtins.toJSON (lib.mapAttrs (name: pkg: {
      binary = pkg.binary;
      version = pkg.version;
      hook = {
        claude-code = "settings.json hooks Notification+Stop";
        codex = "config.toml notify";
        opencode = "plugin ~/.config/opencode/plugins/repose.js";
        gemini-cli = "heuristic";
        pi-coding-agent = "heuristic";
      }.${name};
    }) pkgs.reposeAgents);
  } ];
}
