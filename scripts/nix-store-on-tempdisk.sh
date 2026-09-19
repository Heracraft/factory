#!/usr/bin/env bash
# Put this dev box's /nix on the Azure local temp disk (nvme1n1, 440 GB).
# See docs/ops/DEV-BOX.md for why and for what to do after a deallocate.
# Idempotent: safe to re-run after a stop/start wiped the temp disk.
set -euo pipefail

DISK=/dev/nvme1n1
MNT=/mnt/nixstore
[ "$(id -u)" = 0 ] || { echo "run with sudo"; exit 1; }
[ -b "$DISK" ] || { echo "$DISK not present; is this still a D*lds size?"; exit 1; }

# 1. Filesystem, only if the disk is blank (a fresh temp disk has no label).
if ! blkid -s LABEL -o value "$DISK" | grep -qx nixstore; then
  echo "formatting $DISK"
  mkfs.ext4 -q -L nixstore "$DISK"
fi

# 2. Mount it.
mkdir -p "$MNT"
mountpoint -q "$MNT" || mount "$DISK" "$MNT"
# Single-user Nix: /nix must be owned by the user, and mkfs leaves the
# mount root owned by root. Without this the installer on the recovery
# path refuses "/nix exists but is not writable".
chown "${SUDO_USER:-azureuser}:root" "$MNT"

# 3. First run: copy the existing store over and bind it in place.
if ! mountpoint -q /nix; then
  if [ -d /nix ] && [ -n "$(ls -A /nix 2>/dev/null)" ] && [ ! -e "$MNT/store" ]; then
    echo "copying /nix ($(du -sh /nix | cut -f1)) to $MNT"
    rsync -aHAX /nix/ "$MNT/"
    mv /nix /nix.old
  fi
  mkdir -p /nix
  mount --bind "$MNT" /nix
fi

# 4. Persist across reboots (not across deallocate: the disk comes back empty).
grep -q 'LABEL=nixstore' /etc/fstab || cat >>/etc/fstab <<'FSTAB'
LABEL=nixstore /mnt/nixstore ext4 defaults,nofail,x-systemd.device-timeout=10 0 2
/mnt/nixstore /nix none bind,nofail,x-systemd.requires=/mnt/nixstore 0 0
FSTAB

# 5. After a deallocate the store is empty: reinstall nix (single-user).
if [ ! -x /nix/var/nix/profiles/default/bin/nix ] && [ ! -e "$MNT/store" ]; then
  echo "store is empty (temp disk was wiped); reinstalling nix"
  sudo -u "${SUDO_USER:-azureuser}" sh -c 'curl -L https://nixos.org/nix/install | sh -s -- --no-daemon'
fi

# 6. Verify and clean up.
sudo -u "${SUDO_USER:-azureuser}" bash -lc 'nix store ping && nix eval --expr "1+1"' >/dev/null
df -h /nix | tail -1
[ -d /nix.old ] && { echo "removing /nix.old"; rm -rf /nix.old; }
echo "ok: /nix is on $DISK"
