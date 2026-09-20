# What a host exposes to the personal Grafana server, over WireGuard only:
# node_exporter on <wg0>:9100, the textfile collector storage.nix writes
# into, and hostd's own :9101. Fluent Bit is in fluent-bit.nix
# (docs/workstreams/10-observability.md §2). Nothing here listens on the
# provider NIC.
{ config, lib, pkgs, ... }:
let
  textfileDir = "/var/lib/node_exporter/textfile";
  hostEnv = "/run/repose/host.env";
in
{
  services.prometheus.exporters.node = {
    enable = true;
    port = 9100;
    enabledCollectors = [ "systemd" "textfile" "processes" ];
    extraFlags = [ "--collector.textfile.directory=${textfileDir}" ];
  };

  systemd.services.prometheus-node-exporter = {
    # Bound to the WireGuard address rendered by repose-host-net; the
    # address is bound before wg0 is up thanks to ip_nonlocal_bind.
    unitConfig.ConditionPathExists = hostEnv;
    after = [ "repose-host-net.service" ];
    partOf = [ "repose-host-net.service" ];
    serviceConfig = {
      EnvironmentFile = hostEnv;
      ExecStart = lib.mkForce ''
        ${pkgs.prometheus-node-exporter}/bin/node_exporter \
          --collector.systemd --collector.textfile --collector.processes \
          --collector.textfile.directory=${textfileDir} \
          --web.listen-address=''${WG_ADDR}:9100
      '';
      Restart = "always";
      RestartSec = 5;
    };
  };
}
