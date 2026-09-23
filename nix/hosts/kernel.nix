# Kernel modules, parameters and sysctls a host needs before hostd starts.
# The module list is fixed here instead of locking modules (hardening.nix)
# because hostd creates taps and Cloud Hypervisor opens /dev/kvm after boot.
{ ... }:
{
  boot.kernelModules = [
    "kvm-intel"
    "vhost_vsock"
    "vhost_net"
    "tun"
    "nf_tables"
    "dm_thin_pool"
    "dm_snapshot"
    "overlay"
    "bridge"
    # hostd's per-guest egress policer on the tap's ingress (DECISIONS
    # I-217); sch_htb stays one release for taps shaped before it.
    "sch_htb"
    "sch_ingress"
    "cls_flower"
    "act_police"
    "act_gact"
  ];

  boot.kernelParams = [ "transparent_hugepage=madvise" ];

  boot.kernel.sysctl = {
    "net.ipv4.ip_forward" = 1;
    "fs.inotify.max_user_instances" = 8192;
    # Cloud Hypervisor mmaps guest RAM; the host must not refuse it.
    "vm.overcommit_memory" = 1;
    # sshd and node_exporter bind the wg0 address before wg-quick brings
    # the interface up (network.nix renders both from host.json).
    "net.ipv4.ip_nonlocal_bind" = 1;
    # Bridged guest frames are filtered by the nftables `bridge` family
    # table, not by iptables hooks; keep the bridge from calling into them.
    "net.bridge.bridge-nf-call-iptables" = 0;
    "net.bridge.bridge-nf-call-ip6tables" = 0;
    "net.bridge.bridge-nf-call-arptables" = 0;
  };

  # virtiofsd runs unprivileged in a user namespace (`--sandbox namespace`).
  # The workstream doc names the Debian-only `kernel.unprivileged_userns_clone`
  # sysctl; on the NixOS kernel the equivalent is this option.
  security.allowUserNamespaces = true;
}
