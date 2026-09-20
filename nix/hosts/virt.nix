# Hypervisor plumbing: Cloud Hypervisor, virtiofsd, the users hostd drops
# to, /dev/kvm permissions, and the read-only store export that virtiofsd
# shares with guests.
#
# The export is a read-only bind of /nix/store at /run/repose/store-export
# with an empty tmpfs mounted over `.links`. `.links` is the hard-link farm
# from store optimisation; sharing it would let a guest enumerate every path
# in the store, which is what other tenants have built. A guest can still
# read any path by hash, the same as any public binary cache; the leak being
# closed is enumeration, not access.
{ config, lib, pkgs, ... }:
let
  exportDir = "/run/repose/store-export";
  storeExport = pkgs.writeShellApplication {
    name = "repose-store-export";
    runtimeInputs = [ pkgs.util-linux pkgs.coreutils ];
    text = ''
      # NixOS bind-mounts /nix/store read-only; creating .links needs the
      # same temporary rw remount nix itself uses.
      if [ ! -d /nix/store/.links ]; then
        mount -o remount,bind,rw /nix/store
        mkdir -p /nix/store/.links
        mount -o remount,bind,ro /nix/store
      fi
      mkdir -p ${exportDir}
      if ! mountpoint -q ${exportDir}; then
        mount --bind /nix/store ${exportDir}
      fi
      # The bind starts in /nix/store's peer group (shared propagation), so
      # the tmpfs mounted over .links below would also appear on
      # /nix/store/.links and every store write would fail with EROFS
      # (DECISIONS I-49). Make the export private first.
      mount --make-private ${exportDir}
      mount -o remount,bind,ro,nosuid,nodev ${exportDir}
      if ! mountpoint -q ${exportDir}/.links; then
        mount -t tmpfs -o ro,nosuid,nodev,noexec,size=4k,mode=0555 repose-links-mask ${exportDir}/.links
      fi
      echo "store export ready at ${exportDir}"
    '';
  };
in
{
  environment.systemPackages = [
    pkgs.cloud-hypervisor # ships cloud-hypervisor and ch-remote
    pkgs.virtiofsd
  ];

  users.groups.virtiofsd = { };
  users.users.virtiofsd = {
    isSystemUser = true;
    group = "virtiofsd";
    description = "virtiofsd store share (no write access anywhere under the store)";
  };

  # hostd runs as root; this account exists so taps can be owned by it
  # (`ip tuntap add ... user hostd`, 03-hostd §5.5).
  users.groups.hostd = { };
  users.users.hostd = {
    isSystemUser = true;
    group = "hostd";
    description = "owner of guest tap devices";
  };

  services.udev.extraRules = ''
    KERNEL=="kvm", GROUP="kvm", MODE="0660"
    KERNEL=="vhost-vsock", GROUP="kvm", MODE="0660"
    KERNEL=="vhost-net", GROUP="kvm", MODE="0660"
  '';

  systemd.services.repose-store-export = {
    description = "Read-only /nix/store export for virtiofsd, with .links masked";
    wantedBy = [ "multi-user.target" ];
    before = [ "hostd.service" ];
    unitConfig.DefaultDependencies = false;
    after = [ "local-fs.target" ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStart = "${storeExport}/bin/repose-store-export";
      ExecStop = "${pkgs.writeShellScript "repose-store-export-stop" ''
        ${pkgs.util-linux}/bin/umount -R ${exportDir} || true
      ''}";
    };
  };

  systemd.tmpfiles.rules = [
    "d /run/repose 0755 root root -"
  ];
}
