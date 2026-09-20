# NixOS VM tests for the host configuration (01-host-nixos §7). Each test
# boots a host built from the same modules as the real one, with the
# provider bits off, eth1 as the provider NIC, and the test VM's own root
# disk instead of the disko layout. They run under `nix flake check` and
# need KVM on the builder.
#
#   host-network   the nftables tables and bridge isolation, DHCP on the
#                  provider NIC, repose-host-net from a fixture host.json,
#                  wg0, listeners on wg0 only, Fluent Bit to a real Loki
#   host-storage   the disko data-disk layout on a virtual disk: PV, VG,
#                  thin pool with autoextend, a thin volume, the pool monitor
#   host-services  hostd stub, guests.slice, transient guest survives a
#                  hostd restart and kill, store export with .links masked,
#                  registration with a fixture token (idempotent), sshd on
#                  wg0 with a CA certificate and the PAM audit hook
{ pkgs, nixpkgs, disko, hostModules }:
let
  lib = pkgs.lib;
  system = pkgs.stdenv.hostPlatform.system;
  fixture = ./fixtures/host.json;
  hostId = (builtins.fromJSON (builtins.readFile fixture)).host_id;

  hostNode = { lib, pkgs, ... }: {
    imports = hostModules;
    repose.host = {
      hostName = "host-test";
      provider = "none";
      uplinkInterface = "eth1";
    };
    disko.enableConfig = false;
    # The test driver sets a root password file; the host's locked password
    # would conflict with it.
    users.users.root.hashedPassword = lib.mkForce null;
    # These tests exercise the host's units around hostd with the stub the
    # header describes (registration from a fixture, the audit hook); the
    # flake's hostModules wire the real daemon, which needs an api to
    # register against. The daemon itself is covered by its Go tests.
    repose.host.hostdPackage = lib.mkForce (pkgs.callPackage ../hostd-stub.nix { });
    boot.loader.systemd-boot.enable = lib.mkForce false;
    boot.loader.efi.canTouchEfiVariables = lib.mkForce false;
    virtualisation = {
      memorySize = 3072;
      cores = 2;
      writableStore = true;
      interfaces.eth1 = { vlan = 1; assignIP = false; };
    };
    # No DHCP server in the storage and services tests; do not wait 2 min.
    systemd.network.wait-online.enable = false;
    environment.systemPackages = with pkgs; [ curl iputils netcat-openbsd jq python3 ];
  };

  # The "internet" next to the host: DHCP server for the provider NIC, a
  # web server, an impersonated IMDS address, and a Loki for Fluent Bit.
  inetNode = { pkgs, ... }: {
    virtualisation.interfaces.eth1 = { vlan = 1; assignIP = false; };
    networking.useDHCP = false;
    networking.interfaces.eth1.ipv4.addresses = [
      { address = "203.0.113.9"; prefixLength = 24; }
      { address = "169.254.169.254"; prefixLength = 32; }
    ];
    networking.firewall.enable = false;
    services.dnsmasq = {
      enable = true;
      resolveLocalQueries = false;
      settings = {
        interface = "eth1";
        bind-interfaces = true;
        port = 0;
        dhcp-range = "203.0.113.100,203.0.113.150,12h";
        dhcp-option = [ "option:router,203.0.113.9" ];
      };
    };
    systemd.services.web = {
      wantedBy = [ "multi-user.target" ];
      script = "cd /tmp && exec ${pkgs.python3}/bin/python3 -m http.server 80 --bind 0.0.0.0";
    };
    services.loki = {
      enable = true;
      configFile = "${pkgs.grafana-loki.src}/cmd/loki/loki-local-config.yaml";
    };
    environment.systemPackages = with pkgs; [ grafana-loki curl netcat-openbsd iputils ];
  };

  dataOnly = nixpkgs.lib.nixosSystem {
    inherit system;
    modules = [
      disko.nixosModules.disko
      {
        disko.devices = import ../disko-layout.nix {
          inherit lib;
          osDevice = "/dev/null";
          dataDevice = "/dev/vdb";
          withOs = false;
        };
        disko.enableConfig = false;
      }
    ];
  };
  formatScript = (dataOnly.config.disko.devices._scripts { inherit pkgs; }).formatScript;
in
{
  host-network = pkgs.testers.runNixOSTest {
    name = "repose-host-network";
    nodes.host = hostNode;
    nodes.inet = inetNode;
    testScript = ''
      inet.start()
      host.start()
      inet.wait_for_unit("dnsmasq.service")
      inet.wait_for_unit("web.service")
      inet.wait_for_unit("loki.service")
      inet.wait_for_open_port(3100)

      host.wait_for_unit("multi-user.target")
      host.wait_for_unit("nftables.service")
      host.wait_for_unit("repose-host-net.service")
      host.succeed("journalctl -u repose-host-net --no-pager | grep -q 'no host.json; bridge not configured'")

      with subtest("the provider NIC gets its address by DHCP through the default-drop input chain"):
          host.wait_until_succeeds("ip -4 addr show eth1 | grep -q 'inet 203.0.113.1'", timeout=120)
          host.wait_until_succeeds("ip route | grep -q '^default via 203.0.113.9'")
          host_ip = host.succeed("ip -4 -o addr show eth1 | awk '{print $4}' | cut -d/ -f1").strip()
          host.succeed("curl -sf -m5 http://203.0.113.9/ >/dev/null")
          # the host itself may reach the metadata service (waagent needs it)
          host.succeed("curl -sf -m5 http://169.254.169.254/ >/dev/null")

      with subtest("the repose tables have the documented chains"):
          print(host.succeed("nft list table inet repose"))
          print(host.succeed("nft list table bridge repose"))
          for chain in ["input", "guest_in", "guest_fwd", "guest_dyn", "nat"]:
              host.succeed(f"nft list chain inet repose {chain}")
          host.succeed("nft list chain inet repose guest_fwd | grep -q '169.254.169.254'")
          host.succeed("nft list chain inet repose guest_fwd | grep -q '10.64.0.0/12'")

      with subtest("repose-host-net configures br-guests and wg0 from host.json"):
          host.succeed("mkdir -p /var/lib/repose/hostd && install -m 0600 ${fixture} /var/lib/repose/hostd/host.json")
          host.succeed("systemctl restart repose-host-net.service")
          host.wait_until_succeeds("ip -4 addr show br-guests | grep -q '10.64.4.1/22'")
          print(host.succeed("ip addr show br-guests"))
          host.wait_for_unit("wg-quick-wg0.service")
          print(host.succeed("wg show wg0"))
          host.succeed("wg show wg0 | grep -q 'interface: wg0'")
          host.succeed("ip -4 addr show wg0 | grep -q '10.255.0.7/16'")
          print(host.succeed("cat /run/repose/host.env"))

      # Two guests as network namespaces on the bridge, attached the way
      # hostd attaches taps (host-conventions.md "Network").
      guests = {"ga": ("10.64.4.2", "52:54:00:aa:00:02"), "gb": ("10.64.4.3", "52:54:00:aa:00:03")}
      for ns, (ip, mac) in guests.items():
          host.succeed(" && ".join([
              f"ip netns add {ns}",
              f"ip link add veth-{ns} type veth peer name tap-{ns}",
              f"ip link set tap-{ns} master br-guests up",
              f"bridge link set dev tap-{ns} isolated on learning off flood off",
              f"bridge fdb add {mac} dev tap-{ns} master static",
              f"ip link set veth-{ns} netns {ns}",
              f"ip -n {ns} link set veth-{ns} address {mac}",
              f"ip -n {ns} addr add {ip}/22 dev veth-{ns}",
              f"ip -n {ns} link set lo up",
              f"ip -n {ns} link set veth-{ns} up",
              f"ip -n {ns} route add default via 10.64.4.1",
              f"nft add element bridge repose guests {{ {mac} . {ip} . tap-{ns} }}",
          ]))
      ga = "ip netns exec ga"

      with subtest("a guest reaches the internet through NAT and nothing else"):
          host.succeed(f"{ga} curl -sf -m5 http://203.0.113.9/ >/dev/null")
          host.fail(f"{ga} curl -sf -m3 http://169.254.169.254/")
          host.succeed(f"{ga} ping -c1 -W2 10.64.4.1")
          flood = host.succeed(f"{ga} ping -q -c 40 -i 0.01 -W1 10.64.4.1 || true")
          print(flood)
          assert "40 received" not in flood, "ICMP echo to the host is not rate limited"
          host.fail(f"{ga} ping -c1 -W2 10.64.4.3")
          host.fail(f"{ga} ping -c1 -W2 10.64.8.1")
          host.fail(f"{ga} curl -sf -m3 http://10.64.4.1:9100/")
          host.fail(f"{ga} curl -sf -m3 http://{host_ip}:9100/")
          host.fail(f"{ga} nc -z -w3 10.64.4.1 22")
          host.fail(f"{ga} nc -z -w3 {host_ip} 22")

      with subtest("a guest cannot use another guest's address"):
          host.succeed("ip -n ga addr add 10.64.4.3/22 dev veth-ga")
          host.fail(f"{ga} curl -sf -m3 --interface 10.64.4.3 http://203.0.113.9/")
          host.fail(f"{ga} ping -c1 -W2 -I 10.64.4.3 10.64.4.1")
          host.succeed("ip -n ga addr del 10.64.4.3/22 dev veth-ga")

      with subtest("hostd's counters and set elements survive a ruleset reload"):
          host.succeed("nft add counter inet repose egress-ga")
          host.succeed("nft add rule inet repose guest_dyn ip saddr 10.64.4.2 counter name egress-ga")
          host.succeed(f"{ga} curl -sf -m5 http://203.0.113.9/ >/dev/null")
          before = host.succeed("nft list counter inet repose egress-ga")
          print(before)
          assert "packets 0 " not in before
          host.succeed("systemctl reload nftables.service")
          host.succeed("nft list chain inet repose guest_dyn | grep -q 'counter name \"egress-ga\"'")
          host.succeed("nft list set bridge repose guests | grep -q 'tap-ga'")
          host.succeed(f"{ga} curl -sf -m5 http://203.0.113.9/ >/dev/null")
          host.fail(f"{ga} ping -c1 -W2 10.64.4.3")

      with subtest("sshd and node_exporter listen on wg0 only; nothing on the provider NIC"):
          # grep -c reads to EOF; grep -q would SIGPIPE curl under pipefail and never succeed
          host.wait_until_succeeds("curl -sf -m3 http://10.255.0.7:9100/metrics | grep -c node_exporter_build_info >/dev/null")
          host.wait_until_succeeds("ss -tlnH | grep -q '10.255.0.7:22'")
          print(host.succeed("ss -tlnpH"))
          # column 4 of `ss -tlnH` is the local address:port; everything
          # must be on the WireGuard address or loopback.
          locals_ = host.succeed("ss -tlnH | awk '{print $4}'").split()
          print(locals_)
          for addr in locals_:
              ip = addr.rsplit(":", 1)[0]
              assert ip == "10.255.0.7" or ip.startswith("127.") or ip.startswith("[::1]"), f"listener on {addr}"
          inet.fail(f"curl -sf -m3 http://{host_ip}:9100/")
          inet.fail(f"nc -z -w3 {host_ip} 22")
          inet.succeed(f"ping -c1 -W2 {host_ip}")

      with subtest("Fluent Bit ships journald and console logs to Loki with the documented labels"):
          host.wait_for_unit("fluent-bit.service")
          host.succeed("logger -t repose-test 'repose fluent-bit smoke line'")
          inet.wait_until_succeeds(
              "logcli query --addr http://127.0.0.1:3100 --no-labels '{host=\"${hostId}\"}' | grep -c 'repose fluent-bit smoke line' >/dev/null",
              timeout=180,
          )
          host.succeed("mkdir -p /var/lib/repose/guests/g-test && echo 'guest console smoke line' >> /var/lib/repose/guests/g-test/console.log")
          inet.wait_until_succeeds(
              "logcli query --addr http://127.0.0.1:3100 --no-labels '{component=\"console\",guest_id=\"g-test\"}' | grep -c 'guest console smoke line' >/dev/null",
              timeout=180,
          )
          print(inet.succeed("logcli query --addr http://127.0.0.1:3100 '{component=\"console\"}' --limit 3"))
    '';
  };

  host-storage = pkgs.testers.runNixOSTest {
    name = "repose-host-storage";
    nodes.host = { ... }: {
      imports = [ hostNode ];
      virtualisation.emptyDiskImages = [ 65536 ];
    };
    testScript = ''
      host.wait_for_unit("multi-user.target")
      host.succeed("test -b /dev/vdb")

      with subtest("disko creates the PV, vg-guests and the thin pool"):
          print(host.succeed("${formatScript}"))
          print(host.succeed("lsblk"))
          host.succeed("lsblk | grep -q 'vg--guests-thin'")
          lvs = host.succeed("lvs -a -o lv_name,lv_size,lv_attr,discards,zero,lv_metadata_size --units g vg-guests")
          print(lvs)
          assert "passdown" in lvs
          size_gb = float(host.succeed("lvs --noheadings --units g --nosuffix -o lv_size vg-guests/thin").strip())
          assert 58 <= size_gb <= 62, f"pool is {size_gb} GiB, expected 95 percent of 64 GiB"
          meta_gb = float(host.succeed("lvs --noheadings --units g --nosuffix -o lv_metadata_size vg-guests/thin").strip())
          assert meta_gb >= 1.0, f"pool metadata is {meta_gb} GiB, expected at least 1 GiB"

      with subtest("autoextend is configured and monitored"):
          host.succeed("lvm dumpconfig activation/thin_pool_autoextend_threshold | grep -q '=80'")
          host.succeed("lvm dumpconfig activation/thin_pool_autoextend_percent | grep -q '=10'")
          host.succeed("lvm dumpconfig activation/monitoring | grep -q '=1'")
          host.succeed("systemctl is-active lvm2-monitor.service")

      with subtest("a thin volume can be created, used and snapshotted"):
          host.succeed("lvcreate -V 1G -T vg-guests/thin -n g-test")
          host.succeed("udevadm settle")
          # I-49: guest volumes are group hostd for the unprivileged guest@ unit;
          # anything else in the VG keeps root:disk.
          perm = host.succeed("stat -L -c '%U:%G:%a' /dev/vg-guests/g-test").strip()
          print(f"g-test node {perm}")
          assert perm == "root:hostd:660", f"g-test is {perm}, want root:hostd:660"
          # The guest@ unit's DeviceAllow names the LV symlink; systemd resolves
          # it to the dm node, and DevicePolicy=closed refuses everything else.
          host.succeed("systemd-run --wait --pipe --collect -p User=hostd -p DevicePolicy=closed -p 'DeviceAllow=/dev/vg-guests/g-test rw' -- dd if=/dev/vg-guests/g-test of=/dev/null bs=4k count=1")
          host.fail("systemd-run --wait --pipe --collect -p User=hostd -p DevicePolicy=closed -- dd if=/dev/vg-guests/g-test of=/dev/null bs=4k count=1")
          host.succeed("mkfs.ext4 -q /dev/vg-guests/g-test && mkdir -p /mnt/g && mount /dev/vg-guests/g-test /mnt/g")
          host.succeed("dd if=/dev/urandom of=/mnt/g/blob bs=1M count=64 status=none && umount /mnt/g")
          host.succeed("lvcreate -s -n snap-g-test vg-guests/g-test && lvremove -f vg-guests/snap-g-test")
          # The udev rule matches g-* only: any other volume in the VG keeps root:disk.
          host.succeed("lvcreate -V 1G -T vg-guests/thin -n x-test && udevadm settle")
          other = host.succeed("stat -L -c '%U:%G:%a' /dev/vg-guests/x-test").strip()
          print(f"x-test node {other}")
          assert other == "root:disk:660", f"x-test is {other}, want root:disk:660"
          host.succeed("lvremove -f vg-guests/x-test")
          print(host.succeed("lvs vg-guests"))

      with subtest("the pool monitor exports pool usage for node_exporter"):
          host.succeed("systemctl start repose-pool-monitor.service")
          prom = host.succeed("cat /var/lib/node_exporter/textfile/repose_lvm.prom")
          print(prom)
          assert "repose_lvm_pool_present 1" in prom
          assert "repose_lvm_volumes 1" in prom
          host.succeed("systemctl list-timers --all repose-pool-monitor.timer | grep -q repose-pool-monitor")
    '';
  };

  host-services = pkgs.testers.runNixOSTest {
    name = "repose-host-services";
    nodes.host = { ... }: {
      imports = [ hostNode ];
      # The stub's registration installs this host.json (built in the test
      # with a CA whose private half the test holds) and consumes the token.
      systemd.services.repose-register.environment.HOSTD_STUB_FIXTURE = "/root/host.json";
    };
    testScript = ''
      host.wait_for_unit("multi-user.target")

      with subtest("hostd runs the stub and no sudo exists"):
          host.wait_for_unit("hostd.service")
          host.succeed("journalctl -u hostd --no-pager | grep -q stub_start")
          host.fail("command -v sudo")

      with subtest("the store export is read-only with .links masked"):
          host.wait_for_unit("repose-store-export.service")
          host.succeed("mountpoint -q /run/repose/store-export")
          host.succeed("mountpoint -q /run/repose/store-export/.links")
          assert host.succeed("ls -A /run/repose/store-export/.links").strip() == ""
          assert host.succeed("ls /run/repose/store-export | wc -l").strip() != "0"
          host.fail("touch /run/repose/store-export/x")
          host.fail("touch /run/repose/store-export/.links/x")

      with subtest("a transient guest unit survives hostd restart and kill"):
          host.succeed("systemd-run --unit guest@test --slice guests.slice sleep infinity")
          host.succeed("systemctl is-active guest@test.service")
          host.succeed("systemctl restart hostd.service")
          host.wait_for_unit("hostd.service")
          host.succeed("systemctl is-active guest@test.service")
          host.succeed("systemctl kill --signal=SIGKILL hostd.service")
          host.wait_until_succeeds("systemctl is-active hostd.service")
          host.succeed("systemctl is-active guest@test.service")
          print(host.succeed("systemctl list-units 'guest@*' --no-pager"))
          print(host.succeed("systemctl status hostd.service --no-pager"))
          mm = host.succeed("systemctl show guests.slice -p MemoryMax --value").strip()
          print(f"guests.slice MemoryMax={mm}")
          assert mm not in ("infinity", ""), "guests.slice has no memory cap"
          host.succeed("systemctl stop guest@test.service")

      with subtest("guest@ units run as hostd inside the I-49 sandbox"):
          # The property list is internal/hostd/guest/testdata/unit.golden with
          # this test's ids substituted; the Go golden pins the list itself.
          assert "kvm" in host.succeed("id -nG hostd").split(), "hostd is not in kvm"
          host.succeed("install -d -m 0711 /var/lib/repose/guests")
          host.succeed("install -d -m 1770 -g hostd /var/lib/repose/guests/h2")
          host.succeed("install -d -m 0750 -o virtiofsd -g hostd /var/lib/repose/guests/h2/virtiofsd")
          host.succeed("install -d -m 1770 -g hostd /var/lib/repose/guests/other")
          host.succeed("ip tuntap add dev tap-h2 mode tap user hostd vnet_hdr")
          host.succeed("touch /var/lib/repose/guests/other/vsock.sock && chown hostd /var/lib/repose/guests/other/vsock.sock")
          props = " ".join("-p " + p for p in [
              "MemoryMax=512M", "CPUQuota=100%", "Restart=no", "Slice=guests.slice", "User=hostd",
              "NoNewPrivileges=yes", "CapabilityBoundingSet=", "UMask=0077", "ProtectSystem=strict",
              "ProtectHome=yes", "PrivateTmp=yes", "ProtectKernelTunables=yes", "ProtectKernelModules=yes",
              "ProtectKernelLogs=yes", "ProtectControlGroups=yes", "ProtectProc=invisible",
              "RestrictNamespaces=yes", "RestrictRealtime=yes", "RestrictSUIDSGID=yes", "LockPersonality=yes",
              "SystemCallArchitectures=native", "TemporaryFileSystem=/var/lib/repose/guests",
              "BindPaths=/var/lib/repose/guests/h2", "ReadWritePaths=/var/lib/repose/guests/h2",
              "DevicePolicy=closed", "'DeviceAllow=/dev/kvm rw'", "'DeviceAllow=/dev/net/tun rw'",
              "'RestrictAddressFamilies=AF_UNIX AF_VSOCK'",
          ])
          probe = "/var/lib/repose/guests/h2/probe.py"
          host.succeed("install -m 0644 ${./h2probe.py} " + probe)
          out = host.succeed("systemd-run --wait --pipe --collect --unit guest@h2 " + props + " -- ${pkgs.python3}/bin/python3 " + probe + " 2>&1")
          print(out)
          for line in ["uid " + host.succeed("id -u hostd").strip(), "tap ok", "unix socket ok", "AF_INET refused", "vhost-vsock refused by DevicePolicy"]:
              assert line in out, f"missing {line!r} in probe output"
          ch = host.succeed("systemd-run --wait --pipe --collect --unit guest@h2ch " + props + " -- cloud-hypervisor --version 2>&1")
          print(ch)
          assert "cloud-hypervisor" in ch
          host.succeed("ip link del tap-h2 && rm -rf /var/lib/repose/guests/h2 /var/lib/repose/guests/other")

      with subtest("the nightly snapshot timer is wired to hostd snapshot-all"):
          host.succeed("systemctl list-timers --all repose-snapshot.timer | grep -q repose-snapshot")
          host.succeed("systemctl start repose-snapshot.service")
          host.succeed("journalctl -u repose-snapshot --no-pager | grep -q snapshot_skip")

      with subtest("registration consumes the join token once and is idempotent"):
          host.succeed("test ! -e /var/lib/repose/hostd/host.json")
          host.succeed("ssh-keygen -q -t ed25519 -N \"\" -f /root/ca")
          host.succeed("jq --arg ca \"$(cat /root/ca.pub)\" '.host_ca_pub = $ca' ${fixture} > /root/host.json")
          host.succeed("echo throwaway-join-token > /run/repose/join-token")
          host.succeed("systemctl start repose-register.service")
          host.succeed("test -s /var/lib/repose/hostd/host.json")
          host.succeed("test ! -e /run/repose/join-token")
          print(host.succeed("journalctl -u repose-register --no-pager"))
          host.succeed("journalctl -u repose-register --no-pager | grep -q register_done")
          mtime = host.succeed("stat -c %Y /var/lib/repose/hostd/host.json").strip()
          host.succeed("echo second-token > /run/repose/join-token")
          host.succeed("systemctl start repose-register.service")
          assert host.succeed("systemctl show repose-register.service -p ConditionResult --value").strip() == "no"
          host.succeed("test -e /run/repose/join-token")
          assert host.succeed("stat -c %Y /var/lib/repose/hostd/host.json").strip() == mtime
          host.succeed("rm /run/repose/join-token")

      with subtest("host.json is applied after registration"):
          host.wait_until_succeeds("ip -4 addr show br-guests | grep -q '10.64.4.1/22'")
          host.wait_for_unit("wg-quick-wg0.service")
          host.succeed("grep -q 'ListenAddress 10.255.0.7' /run/repose/sshd.conf")
          host.succeed("cmp /run/repose/host_ca.pub /root/ca.pub")

      with subtest("root logs in over wg0 with a Host CA certificate and the login is audited"):
          host.succeed("ssh-keygen -q -t ed25519 -N \"\" -f /root/op")
          host.succeed("ssh-keygen -q -s /root/ca -I operator-test -n root -V +1h /root/op.pub")
          host.wait_until_succeeds("ss -tlnH | grep -q '10.255.0.7:22'")
          # -n and the redirects keep ssh away from the test driver's console;
          # -F /dev/null skips the system ssh_config, which the client rejects
          # in a test VM because the writable store is not root-owned.
          status, _ = host.execute(
              "ssh -n -v -F /dev/null -i /root/op -o CertificateFile=/root/op-cert.pub -o StrictHostKeyChecking=no "
              "-o UserKnownHostsFile=/dev/null -o BatchMode=yes root@10.255.0.7 true </dev/null >/root/ssh.log 2>&1"
          )
          print(host.succeed("cat /root/ssh.log"))
          if status != 0:
              print(host.succeed("journalctl -u sshd --no-pager | tail -40"))
          assert status == 0, "certificate login over wg0 failed"
          host.wait_until_succeeds("journalctl -t hostd-audit --no-pager | grep -q 'audit_login'")
          print(host.succeed("journalctl -t hostd-audit --no-pager"))
          host.fail(
              "ssh -n -F /dev/null -i /root/op -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "
              "-o BatchMode=yes -o CertificateFile=/dev/null root@10.255.0.7 true </dev/null >/dev/null 2>&1"
          )
    '';
  };
}
