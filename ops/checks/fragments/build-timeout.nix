# The 31-minute build (docs/CHECKLIST.md case (c)): a derivation that sleeps
# past the 30-minute cap. Takes 30 minutes to fail; menu.sh runs it only
# with --with-build-timeout. Expected first line:
#   build timed out after 30 minutes while building sleep-forever-1.0
{ pkgs, ... }:
{
  home.packages = [
    (pkgs.runCommand "sleep-forever-1.0" { } ''
      sleep 1860
      mkdir -p $out
    '')
  ];
}
