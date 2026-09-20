# composeGuest: the platform base plus a user fragment, as one guest
# system (docs/workstreams/12-nix-config-pipeline.md §2). hostd never calls
# this directly: the flake's `guestSystem` output is
#   composeGuest { fragmentPath = "${fragment}/fragment.nix"; }
# with `fragment` overridden per Build (docs/interfaces/nix-build-contract.md,
# DECISIONS I-28), and `nix flake check` composes every example fragment
# through it. The result is mkGuestRunner's package: `share/repose/system`
# is the toplevel hostd boots, `.guestSystem` the nixosSystem, `.toplevel`
# its config.system.build.toplevel.
#
# The workstream doc sketched `{ baseRef, fragment, menuSnippet, guestParams }`.
# baseRef is which checkout hostd evaluates (not an argument of the Nix),
# the menu's system snippet travels inside the fragment as repose.system
# (fragment.nix), and guest parameters are run-time arguments of the runner
# (DECISIONS I-34), so the function takes only what the evaluation needs.
{ mkGuestRunner }:
{ fragment ? null          # a home-manager module value (attrset or function)
, fragmentPath ? null      # or the path of a file holding one
, class ? "large"
, baseVersion ? null
, guestd ? null
, hook ? null
, extraModules ? [ ]
}:
let
  # A path is handed to home-manager as a path, not imported here, so every
  # definition and error carries `fragment.nix:L:C` as its location.
  frag =
    if fragmentPath != null then fragmentPath
    else if fragment != null then fragment
    else { };
in
mkGuestRunner ({
  fragmentModule = frag;
  inherit class extraModules guestd hook;
} // (if baseVersion != null then { inherit baseVersion; } else { }))
