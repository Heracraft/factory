# edge-01: the production edge (infra/azure/modules/edge, DECISIONS I-23,
# I-92). This file holds only what makes it differ from the generic edge
# module, and none of it is secret: the control plane's WireGuard public key
# and the addresses operators connect from. The private key, the gateway's
# certificates and the api CA live on the machine under
# /var/lib/repose/edge/ (docs/ops/RUNBOOK.md "Edge").
#
# The flake's `edge` attribute imports this file rather than having an
# `edge-01` attribute of its own: infra's installer re-runs nixos-anywhere
# when the attribute name changes (infra/azure/modules/edge `triggers`), and
# a live edge must never be reinstalled by a rename.
{ ... }:
{
  repose.edge = {
    # Operators' sshd on 2222: the addresses in infra's `operator_cidrs`
    # (the dev box), next to the WireGuard network the module always allows.
    # The NSG is the outer gate with the same list; this is defence in depth.
    operatorCIDRs = [ "20.102.97.100/32" ];

    # The control VM (coolify-01) dials this hub from its VNet address; its
    # key is /etc/wireguard/publickey there (infra/README.md "Wiring the
    # control plane to the edge"). wgsync keeps it (WG_STATIC_PEERS).
    staticPeers = [
      {
        publicKey = "5Q4fVnx50TZmA+8rSH2l7MFzCZ+dunFwnuClFH6OgXA=";
        allowedIPs = [ "10.255.255.1/32" ];
      }
    ];
  };
}
