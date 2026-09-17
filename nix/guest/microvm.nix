# mkGuest: given a user fragment and project parameters, produce the
# microvm.nix runner package hostd starts. Implemented in
# docs/workstreams/12-nix-config-pipeline.md. Skeleton returns a function
# that errors until implemented, so misuse is loud.
{ nixpkgs, home-manager, microvm, system, overlay, self }:
{ fragment, projectSlug, class, ... }:
throw "nix/guest/microvm.nix: mkGuest not implemented; see docs/workstreams/12-nix-config-pipeline.md"
