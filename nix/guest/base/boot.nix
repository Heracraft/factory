# Kernel and boot for a Cloud Hypervisor direct-boot guest. No bootloader:
# the runner passes the kernel and initrd to the hypervisor. The module list
# is the one docs/workstreams/02-guest-base.md names; the microvm.nix module
# adds its own initrd modules on top when the runner composes this base.
{ config, lib, pkgs, ... }:
{
  # Latest LTS from nixpkgs (linuxPackages is the LTS default).
  boot.kernelPackages = lib.mkDefault pkgs.linuxPackages;

  boot.kernelModules = [
    "overlay"
    "br_netfilter"
    "nf_tables"
    "vsock"
    "vmw_vsock_virtio_transport"
    "virtiofs"
  ];
  boot.initrd.availableKernelModules = [
    "virtio_pci"
    "virtio_blk"
    "virtio_net"
    "virtiofs"
    "overlay"
  ];
  boot.initrd.systemd.enable = lib.mkDefault true;

  boot.loader.grub.enable = false;
  boot.loader.systemd-boot.enable = false;
  boot.loader.efi.canTouchEfiVariables = false;

  # The runner adds console=ttyS0 and the ip= line; these are the same for
  # every guest so they live here.
  boot.kernelParams = [
    "panic=-1"
    "reboot=t"
  ];

  # The guest's whole state is on its thin volume (/), which the kernel sees
  # as the first virtio disk; the shared store is a virtio-fs tag. Both are
  # declared by nix/guest/microvm.nix; the VM tests use the test framework's
  # own layout, which has the same /nix/.ro-store + /nix/.rw-store shape.
  boot.tmp.cleanOnBoot = true;
}
