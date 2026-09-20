# An overlay, a source fetched with a hash (allowed: fixed-output fetches
# run in the build sandbox with network), and a small script package.
{ config, pkgs, lib, ... }:
{
  # Applied to the guest's pkgs before anything is evaluated.
  repose.overlays = [
    (final: prev: {
      # A patched jq is the classic case: the same attribute name, so every
      # use in this profile picks it up.
      jq = prev.jq.overrideAttrs (old: {
        postInstall = (old.postInstall or "") + ''
          echo "repose: jq from the overlay" > $out/share/repose-overlay
        '';
      });
    })
  ];

  home.packages = [
    pkgs.jq
    (pkgs.writeShellScriptBin "repose-hello" ''
      echo "hello from ${config.home.username}"
    '')
    # Something not in nixpkgs, from a release tarball with a hash. The hash
    # is what makes the download allowed; builtins.fetchurl without one is
    # refused at evaluation.
    (pkgs.stdenvNoCC.mkDerivation {
      pname = "repose-example-shell-lib";
      version = "0.1";
      src = pkgs.fetchurl {
        url = "https://raw.githubusercontent.com/NixOS/nixpkgs/b1b875982b17dabde9b4a37f3e229e74913e6db3/COPYING";
        hash = "sha256-yc8GUKaCC1iflqkgYODrk3ECuAj0jNXj813LpEnqCkE=";
      };
      dontUnpack = true;
      installPhase = ''
        mkdir -p $out/share/doc
        cp $src $out/share/doc/nixpkgs-COPYING
      '';
    })
  ];
}
