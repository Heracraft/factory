# The Claude login share (DECISIONS I-278). The host shares one directory
# per user into every guest of that user as the virtio-fs tag claude-auth;
# its only file is Claude Code's .credentials.json, and this unit
# bind-mounts that one file over ~/.claude/.credentials.json. A sign-in in
# any of the user's guests is then a sign-in in all of them, and a token
# refresh in one is seen by the others.
#
# Why one file and a bind mount (experiment A in
# docs/proposals/2026-09-24-claude-login-shared-folder.md): Claude Code
# writes the file as a temp file renamed over it, which replaces a symlink
# with a local file and silently stops sharing; over a bind mount the
# rename fails with EBUSY and Claude Code rewrites the file in place. The
# rest of ~/.claude (settings.json and its hooks) stays per guest, so an
# agent in one project cannot plant a hook that runs in another.
#
# A host with no share for this guest (no user id, an older hostd, the
# share's virtiofsd failed to start) leaves the guest as it was: Claude
# Code keeps its login in the guest. Idempotent: a restart finds the
# mounts in place and changes nothing.
{ config, lib, pkgs, ... }:
let
  user = "dev";
  share = "/run/repose/claude-auth";
  setup = pkgs.writeShellApplication {
    name = "repose-claude-auth";
    runtimeInputs = [ pkgs.coreutils pkgs.util-linux ];
    text = ''
      home=${config.users.users.${user}.home}
      target="$home/.claude/.credentials.json"
      as_dev() { setpriv --reuid=${user} --regid=${user} --init-groups "$@"; }

      mkdir -p ${share}
      if ! findmnt -n --mountpoint ${share} >/dev/null; then
        if ! mount -t virtiofs -o nosuid,nodev,noexec claude-auth ${share} 2>/dev/null; then
          echo "no Claude login share on this host; Claude Code keeps its login in this machine"
          exit 0
        fi
      fi
      # Both files must exist before the bind mount, and as dev: the share
      # maps dev to the host account that owns it.
      [ -e ${share}/.credentials.json ] || as_dev install -m 0600 /dev/null ${share}/.credentials.json
      as_dev mkdir -p "$home/.claude"
      if [ -L "$target" ]; then
        echo "$target is a symlink; not sharing the Claude login into this machine" >&2
        exit 0
      fi
      [ -e "$target" ] || as_dev install -m 0600 /dev/null "$target"
      findmnt -n --mountpoint "$target" >/dev/null || mount --bind ${share}/.credentials.json "$target"
    '';
  };
in
{
  systemd.services.repose-claude-auth = {
    description = "Claude login shared by this user's machines (I-278)";
    wantedBy = [ "multi-user.target" ];
    # Before any login session or dev's user manager (whose tmux runs the
    # agents), so no Claude Code process starts on the unshared file.
    before = [ "systemd-user-sessions.service" "user@${toString config.users.users.${user}.uid}.service" ];
    after = [ "local-fs.target" ];
    unitConfig.DefaultDependencies = false;
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStart = "${setup}/bin/repose-claude-auth";
    };
  };
}
