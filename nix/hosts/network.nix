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
      bootstrap=${if cfg.bootstrap.enable then "1" else "0"}
      mkdir -p "$run"
      umask 022

      write_sshd() {
        # $1: WireGuard address or empty. With no address sshd listens on
        # all interfaces and the nftables input chain is the gate.
        {
          if [ "$bootstrap" = 1 ] || [ -z "$1" ]; then
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
      wg_address=$(jq -er .wg.address "$hostJson")
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

      (
        umask 077
        {
          echo "[Interface]"
          echo "PrivateKey = $(jq -er .wg.private_key "$hostJson")"
          echo "Address = $wg_address"
          echo
          echo "[Peer]"
          echo "PublicKey = $(jq -er .wg.edge_pubkey "$hostJson")"
          echo "Endpoint = $(jq -er .wg.edge_endpoint "$hostJson")"
          echo "AllowedIPs = 10.255.0.0/16"
          echo "PersistentKeepalive = 25"
        } > "$run/wg0.conf.tmp"
        mv "$run/wg0.conf.tmp" "$run/wg0.conf"
      )

      jq -er .host_ca_pub "$hostJson" > "$run/host_ca.pub.tmp"
      mv "$run/host_ca.pub.tmp" "$run/host_ca.pub"

      write_sshd "$wg_ip"

      mkdir -p /run/systemd/network/20-br-guests.network.d
      printf '[Network]\nAddress=%s\n' "$bridge_address" > /run/systemd/network/20-br-guests.network.d/10-address.conf
      networkctl reload
      networkctl reconfigure br-guests

      echo "host.json applied: host_id=$host_id guest_cidr=$guest_cidr bridge=$bridge_address"
      systemctl --no-block reload-or-restart sshd.service
      systemctl --no-block restart wg-quick-wg0.service prometheus-node-exporter.service fluent-bit.service
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
