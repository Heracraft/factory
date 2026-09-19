# NixOS configuration for a repose host: a machine that runs tenants' guests
# under hostd and nothing else. docs/workstreams/01-host-nixos.md is the
# spec; docs/interfaces/host-conventions.md lists every path, device and
# unit name, and each of them is created by one of the modules imported
# here. Built here, deployed with nixos-anywhere; never installed on the
# dev box.
{ config, lib, pkgs, ... }:
let
  cfg = config.repose.host;
in
{
  imports = [
    ./disko.nix
    ./kernel.nix
    ./network.nix
    ./nftables.nix
    ./storage.nix
    ./virt.nix
    ./hostd.nix
    ./gc.nix
    ./observability.nix
    ./registration.nix
    ./ssh.nix
    ./hardening.nix
    ./azure.nix
  ];

  options.repose.host = {
    hostName = lib.mkOption {
      type = lib.types.str;
      default = "repose-host";
      description = "Machine hostname. The host id used everywhere else comes from registration, not from this.";
    };

    provider = lib.mkOption {
      type = lib.types.enum [ "azure" "none" ];
      default = "azure";
      description = ''
        Where the host runs. `azure` enables waagent, cloud-init for the join
        token, and the Hyper-V drivers. `none` is for the VM tests and for a
        future provider module (DECISIONS R3-20: Hetzner is a second module,
        added when it exists).
      '';
    };

    uplinkInterface = lib.mkOption {
      type = lib.types.str;
      default = "eth0";
      description = "The provider NIC: DHCP, default route, NAT egress for guests, and nothing inbound.";
    };

    osDevice = lib.mkOption {
      type = lib.types.str;
      default = "/dev/sda";
      description = "OS disk for the disko layout (GPT, ESP, ext4 root).";
    };

    dataDevice = lib.mkOption {
      type = lib.types.str;
      default = "/dev/disk/azure/scsi1/lun0";
      description = ''
        Data disk that becomes the one PV of `vg-guests`. On Azure this is
        LUN 0 of the managed data disk; workstream 11 passes the value for
        the disk it attached.
      '';
    };

    edgeWireGuardAddress = lib.mkOption {
      type = lib.types.str;
      default = "10.255.0.1";
      description = "The edge's WireGuard address (docs/workstreams/06-gateway-edge.md owns the 10.255.0.0/16 plan). Guests may reach it on the hook-ingest and noVNC relay ports only.";
    };

    apiAddr = lib.mkOption {
      type = lib.types.str;
      default = "api.repose.herakraft.co:443";
      description = ''
        Where hostd registers and holds its gRPC stream. Production is the
        api; before it exists this is the `hostdev` stand-in (DECISIONS
        I-17), for example the edge's public address on 443.
      '';
    };

    apiServerName = lib.mkOption {
      type = lib.types.str;
      default = "";
      description = "TLS server name when it differs from apiAddr's host part (hostd --api-server-name). Empty means the address itself.";
    };

    hostdPackage = lib.mkOption {
      type = lib.types.package;
      default = pkgs.callPackage ./hostd-stub.nix { };
      defaultText = lib.literalExpression "pkgs.callPackage ./hostd-stub.nix { }";
      description = ''
        The hostd binary. Workstream 03 provides the real one as the flake's
        `packages.hostd`; until it is merged this is a stub that logs, sleeps,
        and implements only what the host units call (`register`,
        `audit-login`, `snapshot-all`, `version`).
      '';
    };

    overlayCache = {
      url = lib.mkOption {
        type = lib.types.str;
        default = "";
        description = "Platform overlay binary cache URL, added to substituters. Empty until workstream 12 publishes one.";
      };
      publicKey = lib.mkOption {
        type = lib.types.str;
        default = "";
        description = "Signing key of the overlay cache (`name:base64`). Required when `url` is set.";
      };
    };

    bootstrap = {
      enable = lib.mkEnableOption "operator SSH on the provider NIC with a plain public key, for the time before the edge exists (removed by workstream 11 once WireGuard is up)";
      authorizedKeys = lib.mkOption {
        type = lib.types.listOf lib.types.str;
        default = [ ];
        description = "Operator public keys accepted for root while bootstrap is enabled.";
      };
    };

    timeZone = lib.mkOption {
      type = lib.types.str;
      default = "UTC";
      description = "The host's timezone; `repose-snapshot.timer` fires at 03:00 in it (DESIGN §6).";
    };
  };

  config = {
    assertions = [
      {
        assertion = cfg.overlayCache.url == "" || cfg.overlayCache.publicKey != "";
        message = "repose.host.overlayCache.publicKey must be set when overlayCache.url is set; an unsigned substituter is a supply-chain hole.";
      }
      {
        assertion = !cfg.bootstrap.enable || cfg.bootstrap.authorizedKeys != [ ];
        message = "repose.host.bootstrap.enable without authorizedKeys opens sshd on the provider NIC for nobody.";
      }
    ];

    networking.hostName = cfg.hostName;
    time.timeZone = cfg.timeZone;
    i18n.defaultLocale = "C.UTF-8";

    boot.loader.systemd-boot.enable = true;
    boot.loader.systemd-boot.configurationLimit = 5;
    boot.loader.efi.canTouchEfiVariables = true;
    boot.loader.timeout = 2;

    # The operator toolbox named in docs/ops/RUNBOOK.md and the checklists.
    environment.systemPackages = with pkgs; [
      jq
      iproute2
      nftables
      wireguard-tools
      curl
      htop
      tmux
      lsof
      zstd
    ];

    system.stateVersion = "26.11";
  };
}
