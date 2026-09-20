# Options the platform sets per guest at composition time. Everything a
# guest is told at run time (ip, cid, secrets, project) arrives through the
# kernel command line or guestd instead, so the same closure serves any
# guest of a base version (DECISIONS I-34).
{ lib, ... }:
{
  options.repose = {
    baseVersion = lib.mkOption {
      type = lib.types.str;
      default = "dev";
      description = ''
        The platform base version string written to /etc/repose/base-version
        and used as the NixOS label. The flake sets it from the revision of
        this repository; the api's base_versions row carries the same string.
      '';
    };

    class = lib.mkOption {
      type = lib.types.enum [ "small" "large" "xl" ];
      default = "large";
      description = ''
        The guest's size class, for the runner's vcpu and memory defaults
        and for inspection. Nothing in the system closure depends on it
        (the browser slice ceiling is a percentage of guest memory), so the
        closure a Build produces serves any class (DECISIONS I-43).
      '';
    };
  };
}
