# The `repose` nftables tables from docs/interfaces/host-conventions.md.
#
# inet repose
#   input      policy drop: lo, established, wg0 (ssh, exporters), DHCP and
#              ICMP on the provider NIC, ssh on the provider NIC only while
#              bootstrap is on; frames from guests go to guest_in.
#   guest_in   guests to the host: ICMP echo rate-limited, everything else
#              dropped (there is no DHCP; guests get static addresses).
#   guest_fwd  policy drop: established both ways; the gateway over wg0 to
#              guest sshd; guests out through guest_dyn (hostd's counters)
#              and then to the internet only: IMDS, the Azure wire server,
#              every private range (10.64.0.0/12 is other guests, the rest
#              is the VNet, the WireGuard mesh, link-local) all dropped,
#              except the edge's hook-ingest and noVNC relay ports.
#   guest_dyn  empty at boot; hostd adds `ip saddr <ip> counter name
#              egress-<guest_id>` rules and owns them. Reloading this
#              ruleset flushes the chains declared here and never touches
#              guest_dyn or its counters (see extraDeletions).
#   nat        masquerade guest traffic leaving on the provider NIC.
#
# bridge repose
#   forward    policy drop: no frame is ever switched between two guests.
#   input      frames from a tap to the host: IPv4 and ARP only, and only
#              from the (mac, ip, port) tuple hostd registered in `guests`,
#              so a guest cannot spoof another guest's address or poison
#              the bridge's forwarding table.
{ config, lib, pkgs, ... }:
let
  cfg = config.repose.host;
  uplink = cfg.uplinkInterface;
  edge = cfg.edgeWireGuardAddress;

  # Chains this configuration owns. They are declared (idempotently) and
  # flushed before every reload; hostd's chain and counters survive.
  inetChains = {
    input = "type filter hook input priority filter; policy drop;";
    output = "type filter hook output priority filter; policy accept;";
    guest_fwd = "type filter hook forward priority filter; policy drop;";
    nat = "type nat hook postrouting priority srcnat; policy accept;";
    guest_in = "";
  };
  bridgeChains = {
    forward = "type filter hook forward priority filter; policy drop;";
    input = "type filter hook input priority filter; policy drop;";
  };
  declare = family: name: chains: ''
    table ${family} ${name} {
      ${lib.concatStringsSep "\n  " (lib.mapAttrsToList (c: hook: "chain ${c} { ${hook} }") chains)}
    }
  '';
  flushes = family: name: chains:
    lib.concatStringsSep "\n" (map (c: "flush chain ${family} ${name} ${c}") (lib.attrNames chains));
in
{
  networking.nftables = {
    enable = true;
    flushRuleset = false;
    checkRuleset = true;

    # Applied before the ruleset on every start and reload, and on stop.
    extraDeletions = ''
      ${declare "inet" "repose" inetChains}
      ${flushes "inet" "repose" inetChains}
      ${declare "bridge" "repose" bridgeChains}
      ${flushes "bridge" "repose" bridgeChains}
    '';

    ruleset = ''
      table inet repose {
        # hostd-owned: per-guest egress counter rules. Declared here so it
        # exists from boot; never flushed by a reload.
        chain guest_dyn {
        }

        chain input {
          type filter hook input priority filter; policy drop;
          iifname "lo" accept
          # Guests first: conntrack marks a repeated ICMP echo as
          # established, which would skip the rate limit in guest_in.
          iifname "br-guests" jump guest_in
          ct state established,related accept
          ct state invalid drop

          # Operators and scrapes come over WireGuard only.
          iifname "wg0" tcp dport { 22, 9100, 9101 } accept
          iifname "wg0" icmp type echo-request accept

          # The provider NIC: DHCP, ICMP that keeps TCP working, IPv6 ND.
          iifname "${uplink}" udp sport 67 udp dport 68 accept
          iifname "${uplink}" icmp type { destination-unreachable, time-exceeded, parameter-problem } accept
          iifname "${uplink}" icmp type echo-request limit rate 5/second accept
          iifname "${uplink}" icmpv6 type { destination-unreachable, packet-too-big, time-exceeded, parameter-problem, nd-router-advert, nd-neighbor-solicit, nd-neighbor-advert } accept
          ${lib.optionalString cfg.bootstrap.enable ''
          # bootstrap: operator ssh on the provider NIC until the edge exists
          iifname "${uplink}" tcp dport 22 accept
          ''}
          counter drop
        }

        chain guest_in {
          # A deliberate, rate-limited exception for debugging from a guest.
          icmp type echo-request limit rate 5/second accept
          counter drop
        }

        chain guest_fwd {
          type filter hook forward priority filter; policy drop;
          ct state established,related accept
          ct state invalid drop

          # The edge gateway relays SSH to guests over WireGuard.
          iifname "wg0" oifname "br-guests" ip daddr 10.64.0.0/12 tcp dport 22 accept

          iifname "br-guests" meta nfproto ipv6 counter drop
          iifname "br-guests" jump guest_dyn

          # Azure instance metadata (managed-identity tokens) and the wire
          # server (extension secrets): never from a guest.
          iifname "br-guests" ip daddr { 169.254.169.254, 168.63.129.16 } counter drop
          # Other guests, on this host or any other.
          iifname "br-guests" ip daddr 10.64.0.0/12 counter drop
          # The edge: hook ingest and the noVNC relay, nothing else on the mesh.
          iifname "br-guests" oifname "wg0" ip daddr ${edge} tcp dport { 8443, 6081 } accept
          # Everything private: the VNet, the WireGuard mesh, link-local.
          iifname "br-guests" ip daddr { 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 100.64.0.0/10, 169.254.0.0/16 } counter drop
          iifname "br-guests" oifname "${uplink}" counter accept
          counter drop
        }

        chain nat {
          type nat hook postrouting priority srcnat; policy accept;
          oifname "${uplink}" ip saddr 10.64.0.0/12 masquerade
        }

        chain output {
          type filter hook output priority filter; policy accept;
        }
      }

      table bridge repose {
        # hostd-owned: one element per running guest, added at start and
        # removed at stop: { <mac> . <ip> . tap-<8hex> }.
        set guests {
          type ether_addr . ipv4_addr . ifname
        }

        chain forward {
          type filter hook forward priority filter; policy drop;
          counter drop
        }

        chain input {
          type filter hook input priority filter; policy drop;
          iifname != "tap-*" accept
          ether type arp ether saddr . arp saddr ip . iifname @guests accept
          ether type ip ether saddr . ip saddr . iifname @guests accept
          counter drop
        }
      }
    '';
  };
}
