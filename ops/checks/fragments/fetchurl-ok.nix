# A fixed-output fetch with a hash builds: the sandbox has network for it
# (nix-build-contract.md, 12 §9). The file is nixpkgs' COPYING at the
# platform's locked revision; the hash was taken with `nix store
# prefetch-file` on 2026-09-20.
{ pkgs, ... }:
{
  home.file.".m3-copying".source = pkgs.fetchurl {
    url = "https://raw.githubusercontent.com/NixOS/nixpkgs/b1b875982b17dabde9b4a37f3e229e74913e6db3/COPYING";
    hash = "sha256-yc8GUKaCC1iflqkgYODrk3ECuAj0jNXj813LpEnqCkE=";
  };
}
