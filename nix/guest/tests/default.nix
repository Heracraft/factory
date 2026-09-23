# NixOS VM tests for the guest base (docs/workstreams/02-guest-base.md §7).
# They run on QEMU through the NixOS test driver, whose store layout is the
# same shape as the microvm one (/nix/.ro-store read-only, /nix/.rw-store
# overlay upper), so the overlay logic is tested without Cloud Hypervisor.
# guestd is replaced by a fake that owns /run/repose/hooks.sock and records
# every POST, because workstream 04's binary is not part of this base.
{ pkgs, lib, baseVersion, guestBase, guestd, reposeHook, nixpkgsSource }:
let
  fakeGuestd = pkgs.writers.writePython3Bin "fake-guestd" { } ''
    import json
    import os
    import socketserver
    import grp
    from http.server import BaseHTTPRequestHandler

    SOCK = "/run/repose/hooks.sock"
    LOG = "/run/repose/hooks.log"


    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            n = int(self.headers.get("Content-Length", "0"))
            body = self.rfile.read(n)
            try:
                json.loads(body)
            except ValueError:
                self.send_response(400)
                self.end_headers()
                return
            with open(LOG, "ab") as f:
                f.write(body + b"\n")
            self.send_response(200)
            self.end_headers()

        def log_message(self, *a):
            pass


    class Server(socketserver.UnixStreamServer):
        allow_reuse_address = True


    if os.path.exists(SOCK):
        os.unlink(SOCK)
    srv = Server(SOCK, Handler)
    os.chown(SOCK, 0, grp.getgrnam("dev").gr_gid)
    os.chmod(SOCK, 0o660)
    print("fake guestd listening on " + SOCK, flush=True)
    srv.serve_forever()
  '';

  node = { config, pkgs, lib, ... }: {
    imports = [ guestBase ];
    repose.class = "large";
    repose.applyOverlay = false;
    repose.baseVersion = baseVersion;
    repose.guestd.package = guestd;
    repose.hookPackage = reposeHook;
    systemd.services.guestd.serviceConfig.ExecStart = lib.mkForce "${fakeGuestd}/bin/fake-guestd";
    # The test driver gives root a password file; the guest has none and the
    # test asserts that. The driver's backdoor shell needs no password.
    users.users.root.hashedPasswordFile = lib.mkForce null;
    # The test framework's own network config for eth1; the guest network
    # module expects eth0 from the runner, which does not exist here.
    virtualisation.memorySize = 3072;
    virtualisation.cores = 2;
    virtualisation.diskSize = 8192;
    # ssh-keygen and ssh client for the certificate test.
    environment.systemPackages = [ pkgs.openssh ];
    # A path in the host store but outside the system closure, registered
    # in the VM's database so the pin test can install it offline.
    virtualisation.additionalPaths = [ pkgs.hello ];
  };

  helloImage = pkgs.dockerTools.buildImage {
    name = "repose-hello";
    tag = "test";
    copyToRoot = pkgs.buildEnv { name = "hello-root"; paths = [ pkgs.hello ]; pathsToLink = [ "/bin" ]; };
    config.Cmd = [ "/bin/hello" ];
  };

  claudeStopPayload = builtins.toJSON {
    session_id = "abc"; hook_event_name = "Stop"; transcript_path = "/tmp/transcript.jsonl"; stop_hook_active = false;
  };
  transcript = ''
    {"type":"user","message":{"role":"user","content":"do the thing"}}
    {"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"All tests pass and the change is committed."}]}}
  '';
  userHook = builtins.toJSON {
    hooks.Stop = [ { matcher = ""; hooks = [ { type = "command"; command = "echo user-hook"; } ]; } ];
    permissions.allow = [ "Bash(ls:*)" ];
  };

  # A dynamically linked program as a download would be: interpreter
  # /lib64/ld-linux-x86-64.so.2 and no rpath, so it finds neither the
  # loader nor libz without nix-ld (I-218).
  foreignElf = pkgs.runCommandCC "foreign-elf" {
    nativeBuildInputs = [ pkgs.patchelf ];
    buildInputs = [ pkgs.zlib ];
  } ''
    cat > m.c <<'EOF'
    #include <stdio.h>
    #include <zlib.h>
    int main(void) { printf("foreign ok zlib %s\n", zlibVersion()); return 0; }
    EOF
    $CC m.c -lz -o foreign
    patchelf --set-interpreter /lib64/ld-linux-x86-64.so.2 --remove-rpath foreign
    mkdir -p $out/bin
    cp foreign $out/bin/
  '';

  mkTest = name: attrs: pkgs.testers.runNixOSTest ({ inherit name; } // attrs);

  # The exact scripts the CLI sends for I-198 and I-195, kept in step with
  # the code by internal/cli's TestGuestPartsGolden.
  guestParts = ../../../internal/cli/testdata/guest-parts;
in
{
  guest-base = mkTest "guest-base" {
    nodes.guest = node;
    testScript = ''
      guest.start()
      guest.wait_for_unit("multi-user.target")
      guest.wait_for_unit("guestd.service")
      guest.wait_for_file("/run/repose/hooks.sock")

      with subtest("dev user, sudo, locked root"):
          out = guest.succeed("id dev")
          assert "uid=1000(dev)" in out and "wheel" in out and "docker" in out, out
          guest.succeed("sudo -u dev sudo -n true")
          guest.succeed("sudo -u dev sudo -n -i true")
          status = guest.succeed("passwd -S root")
          assert " L " in status or status.split()[1] in ("L", "LK"), status

      with subtest("sysctls"):
          assert guest.succeed("sysctl -n fs.inotify.max_user_watches").strip() == "1048576"
          assert guest.succeed("sysctl -n fs.inotify.max_user_instances").strip() == "1024"

      with subtest("environment in a login shell"):
          guest.succeed("printf 'TZ=Europe/Berlin\\nREPOSE_PROJECT=todo-app\\n' > /etc/repose/env")
          guest.succeed("install -m 0400 -o dev -g dev /dev/null /run/repose/secrets.env && echo \"export MY_TOKEN='s3cr3t'\" > /run/repose/secrets.env")
          env = guest.succeed("sudo -u dev bash -lc 'echo $TZ $LANG $REPOSE $REPOSE_PROJECT $MY_TOKEN $EDITOR'").strip()
          assert env == "Europe/Berlin C.UTF-8 1 todo-app s3cr3t nvim", env
          path = guest.succeed("sudo -u dev bash -lc 'echo $PATH'")
          assert "/home/dev/.npm-global/bin" in path and "/home/dev/.local/bin" in path, path
          assert guest.succeed("cat /etc/repose/base-version").strip() == "${baseVersion}"

      with subtest("tmux session from project.json"):
          guest.succeed("install -d -o dev -g dev -m 0700 /home/dev/.repose")
          guest.succeed("""echo '{"project_id":"0192e4b0-0000-7000-8000-000000000001","slug":"todo-app","name":"todo-app","tz":"Europe/Berlin","class":"large"}' > /home/dev/.repose/project.json && chown dev:dev /home/dev/.repose/project.json""")
          guest.wait_until_succeeds("sudo -u dev tmux ls | grep -q '^todo-app:'", timeout=60)
          win = guest.succeed("sudo -u dev tmux list-windows -t todo-app -F '#{window_name} #{pane_current_path}'").strip()
          assert win == "shell /home/dev/todo-app", win
          guest.succeed("grep -Eq 'set-clipboard +on' /etc/tmux.conf && grep -Eq 'mouse +on' /etc/tmux.conf && grep -Eq 'history-limit +50000' /etc/tmux.conf")

      with subtest("agent binaries and wrappers"):
          for cmd in ["claude", "opencode", "codex", "gemini", "pi"]:
              guest.succeed(f"test -x /run/current-system/sw/bin/{cmd}")
              guest.succeed(f"grep -q repose-agent-setup /run/current-system/sw/bin/{cmd}")
          for tool in ["node", "pnpm", "python3", "uv", "go", "rustup", "just", "rg", "jq", "gh", "git", "direnv", "starship", "zoxide", "eza", "nvim", "docker", "docker-compose", "chromium", "playwright-mcp", "chrome-devtools-mcp", "repose-hook", "repose-guest-profile", "repose-pin-profile"]:
              guest.succeed(f"test -x /run/current-system/sw/bin/{tool}")

      with subtest("claude hooks merged without clobbering a user hook"):
          guest.succeed("install -d -o dev -g dev /home/dev/.claude")
          guest.succeed("""cat > /home/dev/.claude/settings.json <<'EOF'
      ${userHook}
      EOF
      chown dev:dev /home/dev/.claude/settings.json""")
          guest.succeed("sudo -u dev repose-agent-setup claude")
          settings = guest.succeed("cat /home/dev/.claude/settings.json")
          import json
          s = json.loads(settings)
          stop_cmds = [h["command"] for e in s["hooks"]["Stop"] for h in e["hooks"]]
          notif_cmds = [h["command"] for e in s["hooks"]["Notification"] for h in e["hooks"]]
          assert "echo user-hook" in stop_cmds and "repose-hook" in stop_cmds, stop_cmds
          assert "repose-hook" in notif_cmds, notif_cmds
          assert s["permissions"]["allow"] == ["Bash(ls:*)"], s
          # idempotent
          guest.succeed("sudo -u dev repose-agent-setup claude")
          s2 = json.loads(guest.succeed("cat /home/dev/.claude/settings.json"))
          assert s2 == s, (s, s2)
          mcp = json.loads(guest.succeed("cat /home/dev/.claude.json"))
          assert set(mcp["mcpServers"]) >= {"playwright", "chrome-devtools"}, mcp
          guest.succeed("sudo -u dev repose-agent-setup codex && grep -q 'notify = \\[\"repose-hook\"\\]' /home/dev/.codex/config.toml")
          guest.succeed("sudo -u dev repose-agent-setup opencode && test -s /home/dev/.config/opencode/plugins/repose.js")

      with subtest("repose-hook posts to the socket"):
          guest.succeed("""cat > /tmp/transcript.jsonl <<'EOF'
      ${transcript}
      EOF
      chmod 644 /tmp/transcript.jsonl""")
          guest.succeed("""echo '${claudeStopPayload}' | sudo -u dev REPOSE_HOOK_AGENT=claude repose-hook""")
          hooklog = guest.wait_until_succeeds("cat /run/repose/hooks.log")
          ev = json.loads(hooklog.strip().splitlines()[-1])
          assert ev["agent"] == "claude" and ev["kind"] == "completed", ev
          assert "committed" in ev["summary"], ev
          guest.succeed("""echo '{"agent":"claude","kind":"completed","summary":"t"}' | curl -sf --unix-socket /run/repose/hooks.sock -d @- http://x/ -o /dev/null -w '%{http_code}' | grep -q 200""")
          # a broken payload never fails the caller
          guest.succeed("echo 'not json' | sudo -u dev repose-hook")

      with subtest("sshd accepts the right principal and refuses the wrong one"):
          guest.succeed("ssh-keygen -q -t ed25519 -N ''' -f /root/ca && ssh-keygen -q -t ed25519 -N ''' -f /root/user")
          guest.succeed("install -m 0644 /root/ca.pub /run/repose/user_ca.pub")
          guest.succeed("echo 0192e4b0-0000-7000-8000-000000000001 > /etc/ssh/principals/dev && systemctl reload sshd")
          guest.succeed("ssh-keygen -q -s /root/ca -I 'user:heracraft' -n 0192e4b0-0000-7000-8000-000000000001 -V -1m:+12h /root/user.pub")
          out = guest.succeed("ssh -F /dev/null -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes -o CertificateFile=/root/user-cert.pub -i /root/user dev@127.0.0.1 id")
          assert "uid=1000(dev)" in out, out
          guest.succeed("ssh-keygen -q -s /root/ca -I 'user:other' -n 0192e4b0-ffff-7000-8000-00000000beef -V -1m:+12h /root/user.pub")
          err = guest.fail("ssh -F /dev/null -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes -o CertificateFile=/root/user-cert.pub -i /root/user dev@127.0.0.1 id 2>&1")
          assert "Permission denied" in err, err
          guest.fail("ssh -F /dev/null -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes -i /root/user root@127.0.0.1 id")

      with subtest("store overlay and profile pinning"):
          mounts = guest.succeed("mount | grep -E 'ro-store|rw-store|/nix/store'")
          assert "overlay" in mounts, mounts
          # hello is in the host store (lower). Install it into dev's profile
          # and assert the pin copies its closure into the upper dir.
          hello = "${pkgs.hello}"
          guest.succeed(f"sudo -u dev nix profile install --offline {hello}")
          guest.succeed("systemctl start repose-pin-profile.service")
          upper = guest.succeed("awk '$2 == \"/nix/store\" { print $4 }' /proc/mounts | tr , '\\n' | grep '^upperdir=' | cut -d= -f2").strip()
          # the overlay appears twice (initrd and stage 2 views); one line is enough
          upper = upper.splitlines()[0].removeprefix("/sysroot")
          import os
          name = os.path.basename(hello)
          guest.succeed(f"test -d {upper}/{name}/bin && cmp {upper}/{name}/bin/hello {hello}/bin/hello")
          guest.succeed(f"diff -r {upper}/{name} {hello}")
          guest.succeed("sudo -u dev bash -lc 'hello' | grep -q Hello")

      with subtest("guest profile script"):
          prof = json.loads(guest.succeed("sudo -u dev repose-guest-profile"))
          assert prof["slug"] == "todo-app" and prof["dir"] == "/home/dev/todo-app", prof
          assert prof["base_version"] == "${baseVersion}", prof
          assert prof["desktop"]["running"] is False, prof
    '';
  };

  # Workstream 15 on a real guest base: the timezone part through sudo
  # (I-198), the carried git config and its include precedence (I-195),
  # and the caches' guest side (I-202, I-208): Docker's mirror and the
  # one-time ~/.npmrc line, added only when the cache answers.
  guest-parity = mkTest "guest-parity" {
    nodes.guest = { ... }: {
      imports = [ node ];
      environment.systemPackages = [ pkgs.python3 ];
    };
    testScript = ''
      guest.start()
      guest.wait_for_unit("multi-user.target")
      guest.succeed("printf 'TZ=Europe/Berlin\nREPOSE_PROJECT=todo-app\n' > /etc/repose/env")
      guest.succeed("install -d -o dev -g dev -m 0700 /home/dev/.repose")
      guest.succeed("""echo '{"project_id":"0192e4b0-0000-7000-8000-000000000001","slug":"todo-app","name":"todo-app","tz":"Europe/Berlin","class":"large"}' > /home/dev/.repose/project.json && chown dev:dev /home/dev/.repose/project.json""")
      guest.wait_until_succeeds("sudo -H -u dev tmux ls | grep -q '^todo-app:'", timeout=60)
      guest.succeed("mkdir -p /tmp/p && cp -r ${guestParts}/. /tmp/p && chmod -R u+w /tmp/p && chown -R dev:dev /tmp/p")

      with subtest("I-215: nothing of the system listens where auto-forward would pick it up"):
          listeners = guest.succeed("ss -Hltn")
          print(listeners)
          assert ":5355 " not in listeners, "resolved's LLMNR responder is listening"

      with subtest("the session starts in the project's zone; tmux does not take TZ from the client"):
          assert guest.succeed("sudo -H -u dev tmux show-environment -g TZ").strip() == "TZ=Europe/Berlin"
          assert "TZ" not in guest.succeed("sudo -H -u dev tmux show-options -gv update-environment")

      with subtest("I-198: the carry moves /etc/repose/env and tmux to the laptop's zone"):
          out = guest.succeed("sudo -H -u dev sh -e /tmp/p/tz.sh /tmp/p")
          assert "#tz Asia/Tokyo" in out, out
          env = guest.succeed("cat /etc/repose/env")
          assert "TZ=Asia/Tokyo" in env and "REPOSE_PROJECT=todo-app" in env and "Europe" not in env, env
          assert guest.succeed("stat -c '%U %a' /etc/repose/env").strip() == "root 644"
          assert guest.succeed("sudo -H -u dev tmux show-environment -g TZ").strip() == "TZ=Asia/Tokyo"
          guest.succeed("sudo -H -u dev tmux new-window -d -t todo-app -n clock 'date +%Z > /tmp/zone; sleep 30'")
          guest.wait_until_succeeds("grep -qx JST /tmp/zone")
          # the status clock follows too: its #() job runs with the global
          # environment (I-215); the server's own strftime stays in its
          # start zone
          assert "#(date" in guest.succeed("sudo -H -u dev tmux show-options -gv status-right")
          # a #() job runs only for an attached client: attach one on a pty,
          # and have a job on this session's status line record its zone
          guest.succeed("sudo -H -u dev tmux set-option -t todo-app status-left '#(date +%%Z > /tmp/jobzone)'")
          guest.succeed("sudo -H -u dev env TERM=xterm script -qfc 'tmux attach -t todo-app' /dev/null >/dev/null 2>&1 &")
          guest.wait_until_succeeds("grep -qx JST /tmp/jobzone", timeout=30)
          guest.succeed("sudo -H -u dev tmux set-option -u -t todo-app status-left")
          guest.succeed("sudo -H -u dev tmux detach-client -s todo-app || true")
          assert guest.succeed("sudo -H -u dev bash -lc 'date +%Z'").strip() == "JST"
          # the same zone again changes nothing
          assert "#tz" not in guest.succeed("sudo -H -u dev sh -e /tmp/p/tz.sh /tmp/p")

      with subtest("I-195: the carried config applies and a key set in the guest wins"):
          guest.succeed("sudo -H -u dev git config --global alias.st 'status --short'")
          out = guest.succeed("sudo -H -u dev sh -e /tmp/p/git.sh /tmp/p")
          assert "#dropped git core.pager" in out, out
          listing = guest.succeed("sudo -H -u dev git config --list --show-origin")
          print(listing)
          assert guest.succeed("sudo -H -u dev git config alias.st").strip() == "status --short"
          assert guest.succeed("sudo -H -u dev git config alias.lg").strip() == "log --oneline"
          assert guest.succeed("sudo -H -u dev git config user.email").strip() == "work@corp.example"
          assert "core.pager" not in listing, listing
          head = guest.succeed("head -2 /home/dev/.gitconfig")
          assert head == "[include]\n\tpath = ~/.config/git/repose-carried\n", head
          guest.succeed("sudo -H -u dev sh -e /tmp/p/git.sh /tmp/p")
          assert guest.succeed("grep -c repose-carried /home/dev/.gitconfig").strip() == "1"

      with subtest("I-202: Docker uses the host's mirror"):
          guest.wait_for_unit("docker.service")
          info = guest.succeed("docker info")
          assert "http://10.63.255.254:5000/" in info, info

      with subtest("I-208: ~/.npmrc is left alone while no cache answers"):
          guest.wait_until_succeeds("sudo -H -u dev XDG_RUNTIME_DIR=/run/user/1000 systemctl --user show -p ActiveState --value repose-npm-registry.service | grep -qx inactive", timeout=120)
          guest.fail("test -e /home/dev/.npmrc")
          guest.fail("test -e /home/dev/.repose/npm-registry")

      with subtest("I-208: once the cache answers, one registry line, once"):
          guest.succeed("ip addr add 10.63.255.254/32 dev lo")
          guest.succeed("systemd-run --unit fake-front python3 -m http.server 4873 --bind 10.63.255.254")
          guest.wait_for_open_port(4873, "10.63.255.254")
          guest.succeed("sudo -H -u dev XDG_RUNTIME_DIR=/run/user/1000 systemctl --user start repose-npm-registry.service")
          assert guest.succeed("grep -c '^registry=http://10.63.255.254:4873/$' /home/dev/.npmrc").strip() == "1"
          assert guest.succeed("sudo -H -u dev bash -lc 'npm config get registry'").strip() == "http://10.63.255.254:4873/"
          guest.succeed("sudo -H -u dev XDG_RUNTIME_DIR=/run/user/1000 systemctl --user start repose-npm-registry.service")
          assert guest.succeed("grep -c '^registry=' /home/dev/.npmrc").strip() == "1"
          # a project's own registry still wins
          guest.succeed("sudo -H -u dev sh -c 'mkdir -p /home/dev/proj && echo registry=https://corp.example/npm/ > /home/dev/proj/.npmrc'")
          assert guest.succeed("sudo -H -u dev bash -lc 'cd /home/dev/proj && npm config get registry'").strip() == "https://corp.example/npm/"

      with subtest("I-208: a ~/.npmrc with its own registry is never touched"):
          guest.succeed("rm -f /home/dev/.repose/npm-registry")
          guest.succeed("sudo -H -u dev sh -c 'echo registry=https://user.example/ > /home/dev/.npmrc'")
          guest.succeed("sudo -H -u dev XDG_RUNTIME_DIR=/run/user/1000 systemctl --user start repose-npm-registry.service")
          assert guest.succeed("cat /home/dev/.npmrc").strip() == "registry=https://user.example/"
          assert guest.succeed("cat /home/dev/.repose/npm-registry").strip() == "own"
    '';
  };

  # I-218 and I-219: the C toolchain (cc, cgo), the everyday CLIs, nix-ld
  # running a foreign ELF, the pinned nixpkgs registry, and
  # command-not-found with its install hints.
  guest-devtools = mkTest "guest-devtools" {
    nodes.guest = node;
    testScript = ''
      guest.start()
      guest.wait_for_unit("multi-user.target")

      import shlex

      def dev(cmd):
          return guest.succeed(f"sudo -H -u dev bash -lc {shlex.quote(cmd)}")

      with subtest("I-218: C toolchain, cc links, cgo builds"):
          for tool in ["cc", "gcc", "g++", "c++", "ld", "ar", "make", "cmake", "pkg-config"]:
              dev(f"command -v {tool}")
          guest.succeed("install -d -o dev -g dev /tmp/c /tmp/cgo")
          guest.succeed("""cat > /tmp/c/hello.c <<'EOF'
      #include <stdio.h>
      int main(void) { printf("hello from cc\\n"); return 0; }
      EOF
      cat > /tmp/c/hello.cc <<'EOF'
      #include <iostream>
      int main() { std::cout << "hello from c++" << std::endl; }
      EOF
      cat > /tmp/cgo/main.go <<'EOF'
      package main

      // int add(int a, int b) { return a + b; }
      import "C"
      import "fmt"

      func main() { fmt.Println("cgo says", C.add(40, 2)) }
      EOF
      chown -R dev:dev /tmp/c /tmp/cgo""")
          assert dev("cd /tmp/c && cc -o hello hello.c && ./hello").strip() == "hello from cc"
          assert dev("cd /tmp/c && g++ -o hellocc hello.cc && ./hellocc").strip() == "hello from c++"
          out = dev("cd /tmp/cgo && CGO_ENABLED=1 GOTOOLCHAIN=local GOFLAGS=-mod=mod GOCACHE=/tmp/cgo/cache go build -o cgo main.go && ./cgo")
          assert out.strip() == "cgo says 42", out
          # rustup cannot fetch a toolchain in the sandbox; what it needs
          # from the base is a linker driver named cc.
          dev("command -v cc && command -v rustup")

      with subtest("I-218: everyday CLIs"):
          for tool in ["file", "lsof", "zip", "unzip", "dig", "nslookup", "nc", "sqlite3", "psql", "pg_dump", "openssl", "gpg", "patch", "less", "strace", "rsync", "killall", "readelf"]:
              dev(f"command -v {tool}")
          # psql only: no server binaries on PATH.
          guest.fail("sudo -H -u dev bash -lc 'command -v postgres'")
          guest.fail("sudo -H -u dev bash -lc 'command -v initdb'")

      with subtest("I-218: nix-ld runs a prebuilt foreign ELF"):
          interp = guest.succeed("readelf -l ${foreignElf}/bin/foreign | grep 'program interpreter'")
          assert "/lib64/ld-linux-x86-64.so.2" in interp, interp
          guest.succeed("test -e /lib64/ld-linux-x86-64.so.2")
          out = dev("${foreignElf}/bin/foreign")
          assert out.startswith("foreign ok zlib "), out

      with subtest("I-218: nixpkgs is the base's own, offline"):
          reg = dev("nix registry list")
          print(reg)
          line = [l for l in reg.splitlines() if l.startswith("system flake:nixpkgs ")]
          assert line == ["system flake:nixpkgs path:${nixpkgsSource}"], reg
          assert "nixpkgs=flake:nixpkgs" in dev("echo $NIX_PATH")
          ver = dev("timeout 120 nix eval --raw nixpkgs#hello.version")
          assert ver.strip() == "${pkgs.hello.version}", ver

      with subtest("I-219: nix-locate maps a binary to its attribute"):
          attrs = dev("nix-locate --minimal --no-group --type x --type s --whole-name --at-root /bin/cowsay")
          assert "cowsay.out" in attrs.split(), attrs
          assert dev("nix-locate --minimal --no-group --type x --type s --whole-name --at-root /bin/reposenosuchcommand").strip() == ""

      with subtest("I-219: an unknown command that nixpkgs has prints the hint"):
          out = guest.succeed("sudo -H -u dev bash -ic 'cowsay hi; echo status=$?' 2>&1 || true")
          print(out)
          assert "cowsay is not installed. It is in the nixpkgs package cowsay:" in out, out
          assert "nix profile add nixpkgs#cowsay" in out, out
          assert "repose config add cowsay" in out, out
          assert "status=127" in out, out

      with subtest("I-219: a truly unknown command prints the plain not-found"):
          out = guest.succeed("sudo -H -u dev bash -ic 'reposenosuchcommand; echo status=$?' 2>&1 || true")
          print(out)
          assert "reposenosuchcommand: command not found" in out, out
          assert "nixpkgs#" not in out, out
          assert "status=127" in out, out

      with subtest("I-219: a command being installed says so"):
          guest.succeed("echo cowsay > /run/user/1000/repose-installing && chown dev:dev /run/user/1000/repose-installing")
          out = guest.succeed("sudo -H -u dev bash -ic 'cowsay hi' 2>&1 || true")
          assert "cowsay is still being installed; try again in a moment" in out, out
          assert "nixpkgs#" not in out, out
          guest.succeed("rm /run/user/1000/repose-installing")
    '';
  };

  guest-docker = mkTest "guest-docker" {
    nodes.guest = node;
    testScript = ''
      guest.start()
      guest.wait_for_unit("multi-user.target")
      guest.wait_for_unit("docker.service")
      with subtest("docker runs with overlay2 and the pinned address pool"):
          info = guest.succeed("docker info")
          assert "Storage Driver: overlay2" in info, info
          guest.succeed("docker load < ${helloImage}")
          out = guest.succeed("sudo -u dev docker run --rm repose-hello:test")
          assert "Hello, world!" in out, out
          guest.succeed("docker network create t")
          subnet = guest.succeed("docker network inspect t -f '{{(index .IPAM.Config 0).Subnet}}'").strip()
          assert subnet.startswith("172.2") and int(subnet.split(".")[1]) in range(20, 24), subnet
          guest.succeed("docker compose version")
    '';
  };

  guest-desktop = mkTest "guest-desktop" {
    nodes.guest = node;
    testScript = ''
      guest.start()
      guest.wait_for_unit("multi-user.target")
      guest.wait_for_unit("repose-novnc.socket")
      with subtest("connecting to 6080 starts the chain"):
          guest.succeed("systemctl is-active repose-xvfb.service && exit 1 || true")
          out = guest.succeed("curl -s -m 20 -o /dev/null -w '%{http_code}' http://127.0.0.1:6080/vnc.html")
          assert out.strip() == "200", out
          guest.wait_for_unit("repose-xvfb.service")
          guest.wait_for_unit("repose-x11vnc.service")
          guest.wait_for_unit("repose-novnc.service")
          guest.succeed("test -S /tmp/.X11-unix/X99")
          pw = guest.succeed("cat /run/repose/desktop/vnc-password").strip()
          assert len(pw) == 8, pw
          disp = guest.succeed("sudo -u dev bash -lc 'echo $DISPLAY'").strip()
          assert disp == ":99", disp
          prof = guest.succeed("sudo -u dev repose-guest-profile desktop status").strip()
          assert prof == "running", prof

      with subtest("idle stop"):
          guest.succeed("systemctl start repose-desktop-idle.service")
          for u in ["repose-xvfb", "repose-x11vnc", "repose-novnc", "repose-openbox"]:
              guest.fail(f"systemctl is-active {u}.service")
          guest.succeed("systemctl is-active repose-novnc.socket")
          disp = guest.succeed("sudo -u dev bash -lc 'echo -n $DISPLAY'")
          assert disp == "", disp

      with subtest("idle check stops after the timeout"):
          guest.succeed("curl -s -m 20 -o /dev/null http://127.0.0.1:6080/vnc.html")
          guest.wait_for_unit("repose-xvfb.service")
          guest.succeed("touch -d '-31 minutes' /run/repose/desktop/last-client")
          guest.succeed("systemctl start repose-desktop-idle-check.service")
          guest.fail("systemctl is-active repose-xvfb.service")

      with subtest("headless chromium renders a page"):
          guest.succeed("install -d -o dev -g dev /home/dev/site && echo '<html><body><h1>repose says hello</h1></body></html>' > /home/dev/site/index.html")
          guest.succeed("sudo -u dev bash -lc 'cd /home/dev && chromium --headless --disable-gpu --no-first-run --screenshot=/home/dev/a.png --window-size=800,600 file:///home/dev/site/index.html' 2>&1 | tail -5")
          size = int(guest.succeed("stat -c %s /home/dev/a.png").strip())
          assert size > 5000, size
          guest.copy_from_machine("/home/dev/a.png", "")

      with subtest("MCP servers run from the packaged versions, offline"):
          out = guest.succeed("sudo -u dev bash -lc 'playwright-mcp --version'")
          assert out.strip(), out
          out = guest.succeed("sudo -u dev bash -lc 'chrome-devtools-mcp --version'")
          assert out.strip(), out
          # playwright's browsers are the packaged ones, not a download.
          guest.succeed("sudo -u dev bash -lc 'test -d \"$PLAYWRIGHT_BROWSERS_PATH\" && ls \"$PLAYWRIGHT_BROWSERS_PATH\" | grep -q chromium'")
    '';
  };
}
