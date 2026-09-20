# The edge: the one public entry point to tenant environments
# (docs/workstreams/06-gateway-edge.md §5.1, docs/DESIGN.md §7). It runs:
#
#   * the SSH gateway on :22 (public), which relays a user into their guest;
#   * the WireGuard hub on 51820/udp that every host dials outbound, so the
#     gateway can reach guests without a host having any inbound port;
#   * the preview-proxy stub on :443 (a static page until the feature exists);
#   * the hook-ingest forwarder and the metrics endpoint on the WireGuard
#     address only;
#   * an operator sshd on 2222, key-only, restricted to the operator address
#     list and the WireGuard network.
#
# It is NixOS rather than a Coolify container because it needs raw TCP/UDP
# ports and the kernel's WireGuard, and a Coolify port mapping would cost an
# app its rolling deploys (DECISIONS R4-13). Built here, deployed with
# nixos-anywhere / nixos-rebuild by workstream 11; never installed on the dev
# box.
#
# The secrets and certificates the units reference are placed under
# /var/lib/repose/edge/ by `repose-admin edge init` and workstream 11's
# deploy (§5.1); every unit is conditioned on its files existing, so a fresh
# edge boots and the gateway comes up as soon as they are in place.
{ config, pkgs, lib, ... }:
let
  cfg = config.repose.edge;
  stateDir = "/var/lib/repose/edge";
  tlsDir = "${stateDir}/tls";
in
{
  imports = [ ./disko.nix ];

  options.repose.edge = {
    osDevice = lib.mkOption {
      type = lib.types.str;
      default = "/dev/nvme0n1";
      description = "OS disk for the disko layout: /dev/nvme0n1 on the NVMe-only v7 sizes (DECISIONS I-39); /dev/sda on a SCSI size.";
    };

    gatewayPackage = lib.mkOption {
      type = lib.types.package;
      description = "The gateway binary (flake packages.gateway). Provides `gateway serve` and `gateway wgsync`.";
    };

    apiUrl = lib.mkOption {
      type = lib.types.str;
      default = "https://api.repose.herakraft.co";
      description = "Base URL of the control-plane api the gateway resolves routes and issues certificates through (docs/interfaces/api.md /internal).";
    };

    wgAddress = lib.mkOption {
      type = lib.types.str;
      default = "10.255.0.1";
      description = "The edge's WireGuard address; the hub of the 10.255.0.0/16 plan this workstream owns (§5.1). Hosts get 10.255.0.x; the gateway binds its metrics and hook-ingest listeners here.";
    };

    wgPort = lib.mkOption {
      type = lib.types.port;
      default = 51820;
      description = "WireGuard listen port every host dials.";
    };

    operatorSSHPort = lib.mkOption {
      type = lib.types.port;
      default = 2222;
      description = "Operator sshd port; 22 belongs to the user gateway. Every host provisioner jumps through it (DECISIONS I-23).";
    };

    operatorCIDRs = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      description = "Source ranges allowed to reach the operator sshd on ${toString cfg.operatorSSHPort}, in addition to the WireGuard network. The Azure NSG is the outer gate (workstream 11); this is defence in depth.";
    };

    lokiUrl = lib.mkOption {
      type = lib.types.str;
      default = "";
      description = "Loki push URL Fluent Bit ships the edge journal to, over WireGuard (docs/ops/OBSERVABILITY.md). Empty disables shipping.";
    };
  };

  config = {
    # --- base / boot (Azure v7, NVMe-only; see nix/hosts/azure.nix) ---------
    boot.loader.systemd-boot.enable = true;
    boot.loader.efi.canTouchEfiVariables = true;
    boot.initrd.kernelModules = [ "hv_vmbus" "hv_netvsc" "hv_utils" "hv_storvsc" "pci-hyperv" "nvme" ];
    boot.initrd.availableKernelModules = [ "nvme" "pci-hyperv" ];
    boot.kernelParams = [ "console=ttyS0" "earlyprintk=ttyS0" "rootdelay=300" ];
    networking.usePredictableInterfaceNames = false;
    networking.hostName = lib.mkDefault "repose-edge";
    system.stateVersion = "26.11";
    time.timeZone = "UTC";

    # The edge routes guest traffic between the gateway process and the
    # WireGuard peers, so forwarding is on.
    boot.kernel.sysctl = {
      "net.ipv4.ip_forward" = 1;
      "net.ipv4.conf.all.rp_filter" = lib.mkForce 2; # loose: replies from a guest arrive via wg0, not the default route
    };

    # --- WireGuard hub -----------------------------------------------------
    # Peers are added at runtime by `gateway wgsync` from GET /internal/hosts
    # (§5.6); none are declared here. The private key is placed by workstream
    # 11 / `repose-admin edge init`.
    networking.wireguard.enable = true;
    networking.wireguard.interfaces.wg0 = {
      ips = [ "${cfg.wgAddress}/16" ];
      listenPort = cfg.wgPort;
      privateKeyFile = "${stateDir}/wg.key";
    };
    # wg-quick/wireguard-wg0 must not start before its key exists, or the
    # unit fails on a fresh edge; wgsync (below) is what fills the peers.
    systemd.services."wireguard-wg0".unitConfig.ConditionPathExists = "${stateDir}/wg.key";

    # --- firewall: exactly the ports of §5.1 -------------------------------
    networking.firewall.enable = false; # the nftables table below is the firewall
    networking.nftables.enable = true;
    networking.nftables.ruleset = ''
      table inet repose-edge {
        chain input {
          type filter hook input priority filter; policy drop;
          iifname "lo" accept
          ct state established,related accept
          ct state invalid drop

          # Public: the user SSH gateway, the preview stub, the WireGuard hub.
          tcp dport 22 accept
          tcp dport 443 accept
          udp dport ${toString cfg.wgPort} accept

          # Operator sshd: the WireGuard network and the documented operator
          # addresses only.
          ip saddr 10.255.0.0/16 tcp dport ${toString cfg.operatorSSHPort} accept
          ${lib.concatMapStringsSep "\n          "
            (c: ''ip saddr ${c} tcp dport ${toString cfg.operatorSSHPort} accept'')
            cfg.operatorCIDRs}

          # Guests reach the hook-ingest and noVNC-relay ports over WireGuard.
          iifname "wg0" tcp dport { 8443, 6081 } accept

          # Scrapes over WireGuard only: node_exporter and the gateway's own
          # metrics.
          iifname "wg0" tcp dport { 9100, 9102 } accept
          iifname "wg0" icmp type echo-request accept

          # ICMP that keeps TCP working on the public NIC.
          icmp type { destination-unreachable, time-exceeded, parameter-problem, echo-request } limit rate 5/second accept
          icmpv6 type { destination-unreachable, packet-too-big, time-exceeded, parameter-problem, nd-router-advert, nd-neighbor-solicit, nd-neighbor-advert } accept
          counter drop
        }

        chain forward {
          type filter hook forward priority filter; policy drop;
          # The gateway originates every connection to a guest, so its return
          # traffic is `established`; hosts never route through the edge to
          # one another (that is enforced by per-host AllowedIPs).
          ct state established,related accept
          counter drop
        }

        chain output {
          type filter hook output priority filter; policy accept;
        }
      }
    '';

    # --- the gateway service ------------------------------------------------
    systemd.services.gateway = {
      description = "repose SSH gateway (relay to tenant guests)";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" "wireguard-wg0.service" ];
      wants = [ "network-online.target" ];
      # Needs the mTLS client cert, the api CA and the host key; without them
      # it cannot reach /internal or present a verifiable host certificate.
      unitConfig.ConditionPathExists = [
        "${stateDir}/gateway.crt"
        "${stateDir}/ssh_host_ed25519_key"
      ];
      environment = {
        API_URL = cfg.apiUrl;
        GATEWAY_CLIENT_CERT = "${stateDir}/gateway.crt";
        GATEWAY_CLIENT_KEY = "${stateDir}/gateway.key";
        API_CA = "${stateDir}/api-ca.pem";
        HOST_KEY = "${stateDir}/ssh_host_ed25519_key";
        HOST_CERT = "${stateDir}/ssh_host_ed25519_key-cert.pub";
        GATEWAY_SSH_KEY = "${stateDir}/gateway_ssh_key";
        GATEWAY_LISTEN = ":22";
        METRICS_LISTEN = "${cfg.wgAddress}:9102";
        HOOK_LISTEN = "${cfg.wgAddress}:8443";
        HOOK_TLS_CERT = "${tlsDir}/edge-internal.crt";
        HOOK_TLS_KEY = "${tlsDir}/edge-internal.key";
        PREVIEW_LISTEN = ":443";
        PREVIEW_TLS_CERT = "${tlsDir}/wildcard.crt";
        PREVIEW_TLS_KEY = "${tlsDir}/wildcard.key";
      };
      serviceConfig = {
        ExecStart = "${cfg.gatewayPackage}/bin/gateway serve";
        # 22 and 443 are privileged; DynamicUser needs the capability to bind
        # them. Everything else the gateway touches is a file it reads and a
        # network dial, which need no privilege.
        DynamicUser = true;
        SupplementaryGroups = [ "repose-edge" ];
        AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
        CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
        NoNewPrivileges = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        PrivateTmp = true;
        ReadOnlyPaths = [ stateDir ];
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_UNIX" ];
        Restart = "always";
        RestartSec = 1;
      };
    };

    # --- wgsync: WireGuard peer reconciler ---------------------------------
    # A separate unit so CAP_NET_ADMIN (needed to add peers and routes) is
    # not held by the relay. It loops internally every 30 s (§5.6).
    systemd.services.wgsync = {
      description = "repose WireGuard peer sync from the api host list";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" "wireguard-wg0.service" ];
      wants = [ "network-online.target" ];
      unitConfig.ConditionPathExists = "${stateDir}/gateway.crt";
      path = [ pkgs.wireguard-tools pkgs.iproute2 ];
      environment = {
        API_URL = cfg.apiUrl;
        GATEWAY_CLIENT_CERT = "${stateDir}/gateway.crt";
        GATEWAY_CLIENT_KEY = "${stateDir}/gateway.key";
        API_CA = "${stateDir}/api-ca.pem";
        WG_INTERFACE = "wg0";
      };
      serviceConfig = {
        ExecStart = "${cfg.gatewayPackage}/bin/gateway wgsync";
        DynamicUser = true;
        SupplementaryGroups = [ "repose-edge" ];
        AmbientCapabilities = [ "CAP_NET_ADMIN" ];
        CapabilityBoundingSet = [ "CAP_NET_ADMIN" ];
        NoNewPrivileges = true;
        Restart = "always";
        RestartSec = 5;
      };
    };

    # A group the gateway and wgsync join to read the certificates and keys
    # under stateDir that repose-admin/workstream 11 place there (mode 0640
    # root:repose-edge).
    users.groups.repose-edge = { };
    systemd.tmpfiles.rules = [
      "d ${stateDir} 0750 root repose-edge -"
      "d ${tlsDir} 0750 root repose-edge -"
    ];

    # --- operator sshd on 2222 --------------------------------------------
    # Key-only; the nftables table and the NSG restrict who reaches it. Root
    # is the only account (this is an appliance), certificate or the deploy
    # key. Password auth off, and a verbose log so a probe is recorded
    # (review M-2).
    services.openssh = {
      enable = true;
      ports = [ cfg.operatorSSHPort ];
      openFirewall = false;
      settings = {
        PermitRootLogin = "prohibit-password";
        PasswordAuthentication = false;
        KbdInteractiveAuthentication = false;
        AllowUsers = [ "root" ];
        LogLevel = "VERBOSE";
        X11Forwarding = false;
      };
    };

    # --- observability -----------------------------------------------------
    services.prometheus.exporters.node = {
      enable = true;
      port = 9100;
      listenAddress = cfg.wgAddress;
      enabledCollectors = [ "systemd" "textfile" ];
    };
    systemd.services.prometheus-node-exporter = {
      after = [ "wireguard-wg0.service" ];
      unitConfig.ConditionPathExists = "${stateDir}/wg.key";
    };

    services.fluent-bit = lib.mkIf (cfg.lokiUrl != "") (
      let
        # lokiUrl is scheme://host[:port][/path]; take host and port.
        afterScheme = lib.last (lib.splitString "://" cfg.lokiUrl);
        hostPort = builtins.head (lib.splitString "/" afterScheme);
        lokiHost = builtins.head (lib.splitString ":" hostPort);
        parts = lib.splitString ":" hostPort;
        lokiPort = if lib.length parts > 1 then lib.last parts else "3100";
      in {
        enable = true;
        settings = {
          service.flush = 5;
          pipeline = {
            inputs = [{ name = "systemd"; tag = "edge.*"; read_from_tail = "on"; }];
            outputs = [{
              name = "loki";
              match = "*";
              host = lokiHost;
              port = lokiPort;
              labels = "host=edge";
              line_format = "json";
            }];
          };
        };
      });

    # --- hardening: an appliance, updated by rebuild, never auto ----------
    system.autoUpgrade.enable = false;
    services.fstrim.enable = true;
    documentation.enable = false;
    documentation.nixos.enable = false;
    users.mutableUsers = false;
    users.users.root.hashedPassword = "!";
    users.allowNoPasswordLogin = true;
    services.journald.settings.Journal.SystemMaxUse = "2G";
    services.resolved.settings.Resolve = { LLMNR = "false"; MulticastDNS = "false"; };

    environment.systemPackages = with pkgs; [ wireguard-tools iproute2 ];
  };
}
