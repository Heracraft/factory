# Host networking. systemd-networkd manages the provider NIC (DHCP) and the
# guest bridge; the bridge address and the WireGuard tunnel are configured
# at boot from host.json by `repose-host-net`, because the host's guest /22
# and its WireGuard keys are assigned at registration, not at build time.
#
# repose-host-net renders, from /var/lib/repose/hostd/host.json:
#   /run/repose/host.env                 ids and addresses for other units
#   /run/repose/wg0.conf                 wg-quick config (0600)
#   /run/repose/host_ca.pub              sshd TrustedUserCAKeys
#   /run/repose/sshd.conf                sshd ListenAddress
#   /run/systemd/network/20-br-guests.network.d/10-address.conf
# then restarts the units that read them. Without host.json it logs
# `no host.json; bridge not configured` and exits 0 so the rest of the host
# boots and hostd can register.
{ config, lib, pkgs, ... }:
let
  cfg = config.repose.host;
  hostJson = "/var/lib/repose/hostd/host.json";
  runDir = "/run/repose";

  hostNet = pkgs.writeShellApplication {
    name = "repose-host-net";
    runtimeInputs = [ pkgs.jq pkgs.iproute2 pkgs.systemd pkgs.coreutils ];
    text = ''
      hostJson=${hostJson}
      run=${runDir}
      mkdir -p "$run"
      # 0755 whatever created it first: the join-token delivery once made
      # it 0700 and every guest's unprivileged virtiofsd then failed to
      # reach the store export (DECISIONS I-95).
      chmod 0755 "$run"
      umask 022

      write_sshd() {
        # $1: WireGuard address or empty. With no address sshd listens on
        # all interfaces and the nftables input chain is the gate (operator
        # ssh on the provider NIC only while bootstrap is on). Once
        # registration has given the host a WireGuard address, sshd binds
        # that alone even with bootstrap on: bootstrap is "until
        # registered", so the installer and the token delivery reach a new
        # host over the VNet and nothing does afterwards (DECISIONS I-92).
        {
          if [ -z "$1" ]; then
            echo "ListenAddress 0.0.0.0"
          else
            echo "ListenAddress $1"
          fi
        } > "$run/sshd.conf.tmp"
        mv "$run/sshd.conf.tmp" "$run/sshd.conf"
      }

      if [ ! -s "$hostJson" ]; then
        echo "no host.json; bridge not configured"
        : > "$run/host_ca.pub"
        write_sshd ""
        rm -f "$run/host.env" "$run/wg0.conf"
        rm -rf /run/systemd/network/20-br-guests.network.d
        networkctl reload
        systemctl --no-block reload-or-restart sshd.service
        exit 0
      fi

      host_id=$(jq -er .host_id "$hostJson")
      guest_cidr=$(jq -er .guest_cidr "$hostJson")
      # WireGuard material and the Host CA arrive with registration by the
      # api; `hostdev` (DECISIONS I-17) registers a host without them, and
      # the bridge, sshd and the exporters must still come up (I-40).
      wg_private_key=$(jq -r '.wg.private_key // empty' "$hostJson")
      wg_address=$(jq -r '.wg.address // empty' "$hostJson")
      wg_edge_pubkey=$(jq -r '.wg.edge_pubkey // empty' "$hostJson")
      wg_endpoint=$(jq -r '.wg.edge_endpoint // empty' "$hostJson")
      wg_ip=''${wg_address%/*}
      loki_url=$(jq -r '.loki_url // empty' "$hostJson")

      # Bridge address: network address + 1 with the guest prefix length.
      IFS='./' read -r a b c d prefix <<< "$guest_cidr"
      bridge_ip="$a.$b.$c.$((d + 1))"
      bridge_address="$bridge_ip/$prefix"

      loki_host=""
      loki_port=3100
      if [ -n "$loki_url" ]; then
        rest=''${loki_url#*://}
        rest=''${rest%%/*}
        loki_host=''${rest%%:*}
        case "$rest" in *:*) loki_port=''${rest##*:} ;; esac
      fi

      cat > "$run/host.env.tmp" <<ENV
      HOST_ID=$host_id
      GUEST_CIDR=$guest_cidr
      BRIDGE_ADDR=$bridge_ip
      WG_ADDR=$wg_ip
      LOKI_HOST=$loki_host
      LOKI_PORT=$loki_port
      ENV
      mv "$run/host.env.tmp" "$run/host.env"

      if [ -n "$wg_private_key" ] && [ -n "$wg_address" ] && [ -n "$wg_edge_pubkey" ] && [ -n "$wg_endpoint" ]; then
        (
          umask 077
          {
            echo "[Interface]"
            echo "PrivateKey = $wg_private_key"
            echo "Address = $wg_address"
            echo
            echo "[Peer]"
            echo "PublicKey = $wg_edge_pubkey"
            echo "Endpoint = $wg_endpoint"
            echo "AllowedIPs = 10.255.0.0/16"
            echo "PersistentKeepalive = 25"
          } > "$run/wg0.conf.tmp"
          mv "$run/wg0.conf.tmp" "$run/wg0.conf"
        )
      else
        # wg-quick-wg0.service is conditioned on this file; without it the
        # tunnel stays down and sshd listens on every interface behind the
        # nftables input chain (operator ssh only while bootstrap is on).
        echo "no WireGuard material in host.json; wg0 not configured"
        rm -f "$run/wg0.conf"
      fi

      # An empty CA file trusts nobody; the bootstrap key still works.
      jq -r '.host_ca_pub // empty' "$hostJson" > "$run/host_ca.pub.tmp"
      mv "$run/host_ca.pub.tmp" "$run/host_ca.pub"

      write_sshd "$wg_ip"

      mkdir -p /run/systemd/network/20-br-guests.network.d
      printf '[Network]\nAddress=%s\n' "$bridge_address" > /run/systemd/network/20-br-guests.network.d/10-address.conf
      networkctl reload
      networkctl reconfigure br-guests

      echo "host.json applied: host_id=$host_id guest_cidr=$guest_cidr bridge=$bridge_address"
      systemctl --no-block reload-or-restart sshd.service
      systemctl --no-block restart prometheus-node-exporter.service fluent-bit.service
      if [ -s "$run/wg0.conf" ]; then
        systemctl --no-block restart wg-quick-wg0.service
      else
        systemctl --no-block stop wg-quick-wg0.service
      fi
    '';
  };
in
{
  networking.useNetworkd = true;
  networking.useDHCP = false;
  networking.dhcpcd.enable = false;
  networking.firewall.enable = false; # nftables.nix is the firewall
  networking.nftables.enable = true;

  systemd.network.enable = true;
  systemd.network.wait-online.anyInterface = true;

  systemd.network.networks."10-uplink" = {
    matchConfig.Name = cfg.uplinkInterface;
    networkConfig = {
      DHCP = "ipv4";
      IPv6AcceptRA = true;
    };
    linkConfig.RequiredForOnline = "routable";
  };

  systemd.network.netdevs."20-br-guests" = {
    netdevConfig = {
      Kind = "bridge";
      Name = "br-guests";
    };
    # No STP: taps never form loops, and a listening delay would add
    # seconds to every guest start. Guest-to-guest forwarding is stopped by
    # the nftables bridge table and by hostd setting every tap `isolated on`.
    bridgeConfig.STP = false;
  };

  systemd.network.networks."20-br-guests" = {
    matchConfig.Name = "br-guests";
    networkConfig = {
      ConfigureWithoutCarrier = true;
      DHCP = "no";
      LinkLocalAddressing = "no";
      IPv6AcceptRA = false;
    };
    linkConfig.RequiredForOnline = "no";
  };

  # hostd creates taps and attaches them itself; wg-quick owns wg0.
  systemd.network.networks."30-unmanaged" = {
    matchConfig.Name = "tap-* wg0";
    linkConfig.Unmanaged = true;
  };

  systemd.services.repose-host-net = {
    description = "Configure br-guests, wg0 and per-host files from host.json";
    wantedBy = [ "multi-user.target" ];
    after = [ "systemd-networkd.service" "nftables.service" ];
    wants = [ "systemd-networkd.service" ];
    before = [ "hostd.service" "wg-quick-wg0.service" "sshd.service" ];
    path = [ pkgs.iproute2 ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStart = "${hostNet}/bin/repose-host-net";
    };
  };

  networking.wg-quick.interfaces.wg0 = {
    configFile = "${runDir}/wg0.conf";
    autostart = true;
  };

  systemd.services.wg-quick-wg0 = {
    unitConfig.ConditionPathExists = "${runDir}/wg0.conf";
    after = [ "repose-host-net.service" ];
    partOf = [ "repose-host-net.service" ];
  };

  environment.systemPackages = [ hostNet ];
}
