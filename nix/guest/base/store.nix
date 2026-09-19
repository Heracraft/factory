# The guest's own nix: a daemon that installs into the writable overlay
# (/nix/.rw-store) with cache.nixos.org as the only substituter, plus
# repose-pin-profile, which copies the closure of dev's profile up into the
# overlay so a host garbage collection of a path the profile shares with an
# old base can never break the guest (docs/workstreams/02-guest-base.md
# "The shared store and the overlay").
#
# Copy-up is done by touching every file of every closure path with the
# mtime the store already gives them (epoch 1): overlayfs copies a file up
# on any attribute change, so this duplicates the bytes into the upper
# directory without changing anything visible. The upper directory is read
# from /proc/mounts, so the same script works for the microvm layout
# (/nix/.rw-store/store) and the NixOS test layout (/nix/.rw-store/upper).
{ config, lib, pkgs, ... }:
let
  pin = pkgs.writeShellApplication {
    name = "repose-pin-profile";
    runtimeInputs = [ pkgs.nix pkgs.coreutils pkgs.findutils pkgs.gnugrep pkgs.gawk ];
    text = ''
      upper=$(awk '$2 == "/nix/store" && $3 == "overlay" { print $4 }' /proc/mounts \
        | tr ',' '\n' | grep '^upperdir=' | head -n1 | cut -d= -f2-)
      if [ -z "$upper" ]; then
        echo "repose-pin-profile: /nix/store is not an overlay; nothing to pin" >&2
        exit 0
      fi
      pinned=0
      for profile in \
        /home/dev/.local/state/nix/profiles/profile \
        /nix/var/nix/profiles/per-user/dev/profile \
        /home/dev/.nix-profile; do
        [ -e "$profile" ] || continue
        target=$(readlink -f "$profile") || continue
        # Every path in the profile's closure that is not yet in the upper
        # dir is copied up. Paths already there (installed by the guest's
        # own daemon) are skipped by the -e test.
        for p in $(nix-store -qR "$target" 2>/dev/null); do
          name=$(basename "$p")
          if [ -e "$upper/$name" ]; then
            continue
          fi
          find "$p" -exec touch -h -d @1 {} + 2>/dev/null || true
          pinned=$((pinned + 1))
        done
      done
      echo "repose-pin-profile: pinned $pinned store paths into $upper"
    '';
  };
in
{
  nix = {
    enable = true;
    package = pkgs.nix;
    settings = {
      experimental-features = [ "nix-command" "flakes" ];
      substituters = [ "https://cache.nixos.org" ];
      trusted-public-keys = [ "cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY=" ];
      sandbox = true;
      auto-optimise-store = false;
      # The guest builds inside its own vcpu/RAM caps; a user's nix build
      # affects only their guest.
      max-jobs = "auto";
      cores = 0;
      trusted-users = [ "root" ];
      allowed-users = [ "root" "dev" ];
    };
    # Never GC inside the guest: the overlay upper is the only copy of what
    # the user installed, and the host owns the shared paths.
    gc.automatic = false;
    optimise.automatic = false;
  };

  environment.systemPackages = [ pin ];

  # At every activation (boot and switch) and whenever dev's profile
  # changes, pin. The path unit fires on `nix profile install`.
  system.activationScripts.repose-pin-profile = {
    deps = [ "users" ];
    text = ''
      ${pin}/bin/repose-pin-profile || true
    '';
  };

  systemd.services.repose-pin-profile = {
    description = "repose: copy dev's profile closure into the store overlay";
    serviceConfig = {
      Type = "oneshot";
      ExecStart = "${pin}/bin/repose-pin-profile";
      Nice = 10;
      IOSchedulingClass = "idle";
    };
  };

  systemd.paths.repose-pin-profile = {
    description = "repose: watch dev's nix profile for changes";
    wantedBy = [ "multi-user.target" ];
    pathConfig = {
      PathChanged = [
        "/home/dev/.local/state/nix/profiles"
        "/nix/var/nix/profiles/per-user/dev"
      ];
      Unit = "repose-pin-profile.service";
    };
  };

  systemd.tmpfiles.rules = [
    "d /home/dev/.local/state 0755 dev dev -"
    "d /home/dev/.local/state/nix 0755 dev dev -"
    "d /home/dev/.local/state/nix/profiles 0755 dev dev -"
    "d /nix/var/nix/profiles/per-user/dev 0755 dev dev -"
  ];
}
