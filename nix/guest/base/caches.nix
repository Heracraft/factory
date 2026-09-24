# The host's npm and Docker Hub caches (DECISIONS I-202, I-208), at the host
# services address every host serves them on (nix/hosts/caches.nix,
# host-conventions.md "Caches").
#
# Docker: the daemon's registry-mirrors. dockerd falls back to Docker Hub by
# itself when the mirror fails, and a login or a private registry goes
# direct as it always does.
#
# npm, pnpm and yarn: one `registry=` line in ~/.npmrc, added once by
# repose-npm-registry (a user unit of dev) and only when the cache answers.
# Not npm_config_registry: npm lets that variable beat a project's own
# .npmrc, so a project with its own registry would stop working (npm
# 11.19), and pnpm 11 reads its registry from .npmrc files only. A
# ~/.npmrc that already names a registry or holds a token for
# registry.npmjs.org is left alone: those users go direct. Deleting the
# line keeps it deleted (the marker remembers it was added).
{ config, lib, pkgs, ... }:
let
  cfg = config.repose.caches;
  npmURL = "http://${cfg.address}:${toString cfg.npmPort}/";
  npmRegistry = pkgs.writeShellApplication {
    name = "repose-npm-registry";
    runtimeInputs = [ pkgs.curl pkgs.gnugrep pkgs.coreutils ];
    text = ''
      f="$HOME/.npmrc"
      m="$HOME/.repose/npm-registry"
      if [ -e "$m" ]; then
        exit 0
      fi
      mkdir -p "$HOME/.repose"
      if [ -f "$f" ] && grep -Eq '^[[:space:]]*registry[[:space:]]*=|registry\.npmjs\.org/:_' "$f"; then
        echo "$f names its own registry or a registry.npmjs.org token; leaving it alone"
        echo own > "$m"
        exit 0
      fi
      # Any answer from the front means it is there; 000 is no answer.
      for _ in 1 2 3 4 5 6; do
        code=$(curl -s -m 3 -o /dev/null -w '%{http_code}' ${npmURL}-/ping || true)
        if [ "$code" != "000" ] && [ -n "$code" ]; then
          printf '# repose: this host'"'"'s npm cache (DECISIONS I-202). Delete these two lines to use the registry directly.\nregistry=%s\n' ${npmURL} >> "$f"
          echo added > "$m"
          echo "npm registry set to the host's cache"
          exit 0
        fi
        sleep 10
      done
      echo "the host's npm cache did not answer; $f unchanged, tried again at the next boot"
    '';
  };
in
{
  options.repose.caches = {
    enable = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = "Use the host's npm and Docker Hub caches.";
    };
    address = lib.mkOption {
      type = lib.types.str;
      default = "10.63.255.254";
      description = "The host services address (nix/hosts/caches.nix `repose.host.caches.address`).";
    };
    npmPort = lib.mkOption { type = lib.types.port; default = 4873; };
    dockerPort = lib.mkOption { type = lib.types.port; default = 5000; };
  };

  config = lib.mkIf cfg.enable {
    virtualisation.docker.daemon.settings = {
      registry-mirrors = [ "http://${cfg.address}:${toString cfg.dockerPort}" ];
      insecure-registries = [ "${cfg.address}:${toString cfg.dockerPort}" ];
    };

    systemd.user.services.repose-npm-registry = {
      description = "repose: point npm at the host's cache, once";
      wantedBy = [ "default.target" ];
      unitConfig.ConditionUser = "dev";
      # Outside default.target's ordering: a target waits for the units it
      # wants unless they have no default dependencies, and the project's
      # tmux session (so hostd's SetupProject, so every start) waits for
      # default.target. With the cache not answering, this retries for about
      # a minute, and every start waited that minute (DECISIONS I-231).
      unitConfig.DefaultDependencies = false;
      conflicts = [ "shutdown.target" ];
      before = [ "shutdown.target" ];
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${npmRegistry}/bin/repose-npm-registry";
      };
    };
  };
}
