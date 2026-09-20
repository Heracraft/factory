# Over the 20 GB closure cap without downloading 20 GB: one derivation whose
# output is a 21 GB sparse file. The NAR size (what `nix path-info -S`
# sums) is the logical size, so the check fires with `closure_too_large`
# and the ten largest paths; the store holds only the sparse blocks and the
# result gets no GC root (nix-build-contract.md step 5).
{ pkgs, ... }:
{
  home.packages = [
    (pkgs.runCommand "m3-twenty-one-gb" { } ''
      mkdir -p $out/share/m3
      truncate -s 21G $out/share/m3/big
    '')
  ];
}
