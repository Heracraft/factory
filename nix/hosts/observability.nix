# What a host exposes to the personal Grafana server, over WireGuard only:
# node_exporter on <wg0>:9100 (hostd serves :9101 itself), the textfile
# collector storage.nix writes into, and Fluent Bit shipping journald plus
# every guest's console log to Loki with labels host, component and, for
# console lines, guest_id (docs/ops/OBSERVABILITY.md). Nothing here listens
# on the provider NIC.
{ config, lib, pkgs, ... }:
let
  textfileDir = "/var/lib/node_exporter/textfile";
  hostEnv = "/run/repose/host.env";

  # component: the unit name without .service for host units, `console`
  # for guest console logs, plus guest_id parsed from the tail path.
  labelsLua = pkgs.writeText "repose-labels.lua" ''
    function host_unit(tag, ts, record)
      local unit = record["_SYSTEMD_UNIT"] or record["SYSLOG_IDENTIFIER"] or "kernel"
      unit = string.gsub(unit, "%.service$", "")
      record["component"] = unit
      record["unit"] = unit
      return 2, ts, record
    end

    function console(tag, ts, record)
      local path = record["path"] or ""
      local id = string.match(path, "/guests/([^/]+)/console%.log")
      record["component"] = "console"
      if id ~= nil then record["guest_id"] = id end
      record["path"] = nil
      return 2, ts, record
    end
  '';
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

  services.fluent-bit = {
    enable = true;
    settings = {
      service = {
        flush = 5;
        grace = 10;
        log_level = "info";
        "storage.path" = "/var/lib/fluent-bit/storage";
        "storage.sync" = "normal";
        "storage.max_chunks_up" = 64;
        "storage.backlog.mem_limit" = "64M";
      };
      pipeline = {
        inputs = [
          {
            name = "systemd";
            tag = "host.*";
            read_from_tail = "on";
            strip_underscores = "off";
            db = "/var/lib/fluent-bit/journald.db";
            "storage.type" = "filesystem";
          }
          {
            name = "tail";
            tag = "console.*";
            path = "/var/lib/repose/guests/*/console.log";
            path_key = "path";
            db = "/var/lib/fluent-bit/console.db";
            # A console log is wanted from its first line; the db keeps the
            # offset so nothing is shipped twice.
            read_from_head = "on";
            refresh_interval = 5;
            rotate_wait = 10;
            skip_long_lines = "on";
            "storage.type" = "filesystem";
          }
        ];
        filters = [
          {
            name = "lua";
            match = "host.*";
            script = "${labelsLua}";
            call = "host_unit";
          }
          {
            name = "lua";
            match = "console.*";
            script = "${labelsLua}";
            call = "console";
          }
          {
            # Keep the journal fields that carry diagnostic value; drop the
            # rest (cmdline, uid, and everything else the never-log list
            # forbids or that is noise as a JSON line).
            name = "record_modifier";
            match = "host.*";
            allowlist_key = [ "MESSAGE" "PRIORITY" "component" "unit" "_PID" "SYSLOG_IDENTIFIER" ];
          }
        ];
        outputs = [
          {
            name = "loki";
            match = "*";
            host = "\${LOKI_HOST}";
            port = "\${LOKI_PORT}";
            labels = "host=\${HOST_ID}";
            label_keys = "$component,$guest_id";
            line_format = "json";
            drop_single_key = "off";
            "storage.total_limit_size" = "1G";
            retry_limit = "no_limits";
          }
        ];
      };
    };
  };

  systemd.services.fluent-bit = {
    unitConfig.ConditionPathExists = hostEnv;
    after = [ "repose-host-net.service" "wg-quick-wg0.service" ];
    partOf = [ "repose-host-net.service" ];
    serviceConfig = {
      EnvironmentFile = hostEnv;
      StateDirectory = "fluent-bit";
      # Console logs are written by hostd (root) under a 0700 directory;
      # read-only search is all the shipper needs.
      AmbientCapabilities = [ "CAP_DAC_READ_SEARCH" ];
      CapabilityBoundingSet = [ "CAP_DAC_READ_SEARCH" ];
    };
  };
}
