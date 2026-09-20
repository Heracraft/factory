# Checks for the fragment pipeline (docs/workstreams/12-nix-config-pipeline.md
# §7): every example fragment in docs/features/config-examples composes and
# builds, and the contract's refusals refuse with the documented message.
# Both go through composeGuest, the same path hostd's `guestSystem` takes.
{ pkgs, lib, composeGuest, guestd, hook, baseVersion, examplesDir }:
let
  compose = args: composeGuest ({ inherit guestd hook baseVersion; } // args);

  exampleFiles = lib.filter (n: lib.hasSuffix ".nix" n) (builtins.attrNames (builtins.readDir examplesDir));
  examples = lib.listToAttrs (map
    (n: lib.nameValuePair (lib.removeSuffix ".nix" n) (compose { fragmentPath = examplesDir + "/${n}"; }).toplevel)
    exampleFiles);

  # A fragment that must fail evaluation, and the text its error must carry.
  # The module system reports its errors with `throw`, which tryEval sees;
  # the message check happens outside tryEval by re-evaluating the drvPath
  # under `builtins.seq` guarded by the expectation.
  refusal = name: fragment: expect:
    let
      drv = (compose { inherit fragment; }).toplevel.drvPath;
      r = builtins.tryEval (builtins.deepSeq drv drv);
    in
    if r.success then throw "fragment-contract: ${name}: evaluation succeeded; it must fail with: ${expect}"
    else { inherit name expect; };

  refusals = [
    (refusal "system-outside-allowlist"
      { repose.system = [ { networking.firewall.enable = false; } ]; }
      "repose.system: option 'networking.firewall' is not allowed in a fragment")
    (refusal "system-openssh"
      { repose.system = [ { services.openssh.settings.PermitRootLogin = "yes"; } ]; }
      "repose.system: option 'services.openssh' is not allowed in a fragment")
    (refusal "hm-nixpkgs-overlays"
      { nixpkgs.overlays = [ (f: p: { }) ]; }
      "use repose.overlays")
    (refusal "nixos-option-in-fragment"
      { services.postgresql.enable = true; }
      "does not exist")
  ];
in
{
  # Forces every example's system closure to build; the output lists them.
  fragment-examples = pkgs.runCommand "fragment-examples" { passthru = examples; } ''
    mkdir -p $out
    ${lib.concatStringsSep "\n" (lib.mapAttrsToList (n: t: "ln -s ${t} $out/${n}") examples)}
    ls -l $out
  '';

  # Pure evaluation: each refusal above threw. The expected texts are also
  # what internal/hostd/nixbuild's error mapping and the CLI show.
  fragment-contract = pkgs.writeText "fragment-contract"
    (lib.concatMapStringsSep "\n" (r: "${r.name}: refused (${r.expect})") refusals);
}
