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

  # Downloads the compat test runs offline (I-228): Prisma's
  # debian-openssl-3.0.x schema engine for 6.16's engines commit, which is
  # what the redirect sends a linux-nixos request to, and a manylinux numpy
  # wheel that needs libstdc++ from outside the nixpkgs python.
  prismaCommit = "1c57fdcd7e44b29b9313256c76699e91c3ac3c43";
  prismaSchemaEngineGz = pkgs.fetchurl {
    url = "https://binaries.prisma.sh/all_commits/${prismaCommit}/debian-openssl-3.0.x/schema-engine.gz";
    sha256 = "ee38b431ac7281e87cfbf3b940cdf40d76c6f0f9b06daa9948bb950de472cb56";
  };
  numpyWheel = pkgs.fetchurl {
    url = "https://files.pythonhosted.org/packages/51/64/7de3c91e821a2debf77c92962ea3fe6ac2bc45d0778c1cbe15d4fce2fd94/numpy-2.3.3-cp312-cp312-manylinux_2_27_x86_64.manylinux_2_28_x86_64.whl";
    sha256 = "d9192da52b9745f7f0766531dcfa978b7763916f158bb63bdb8a1eca0068ab20";
  };
  numpyWheelName = "numpy-2.3.3-cp312-cp312-manylinux_2_27_x86_64.manylinux_2_28_x86_64.whl";
  userBinDirsForTest = import ../base/user-bin-dirs.nix;

  # guest-devtools' stand-in agent (I-241): wrapped exactly like the five,
  # it records what environment the wrapper handed it. repose-probe-0 is
  # the first user bin dir's probe program from the I-227 subtest.
  envProbeAgent = (import ../../overlay/agents/wrap.nix { inherit pkgs; }) {
    name = "repose-env-probe";
    pkg = pkgs.writeShellScriptBin "repose-env-probe" ''
      {
        echo "project=''${REPOSE_PROJECT:-}"
        echo "secret=''${MY_TOKEN:-}"
        echo "agent=''${REPOSE_HOOK_AGENT:-}"
        if command -v repose-probe-0 >/dev/null 2>&1; then echo path=ok; fi
        echo done
      } > /tmp/out-agent 2>&1
      sleep 30
    '';
  };

  mkTest = name: attrs: pkgs.testers.runNixOSTest ({ inherit name; } // attrs);

  # The exact scripts the CLI sends for I-198 and I-195, kept in step with
  # the code by internal/cli's TestGuestPartsGolden.
  guestParts = ../../../internal/cli/testdata/guest-parts;

  # guest-tools-carry's stand-ins. fakeNixpkgs is a flake with three
  # packages, each one script copied into $out/bin; nothing to download.
  fakeBin = name: text: pkgs.writeScript name "#!${pkgs.bash}/bin/bash\n${text}\n";
  fakeNode = fakeBin "node" ''if [ "''${1:-}" = --version ]; then echo v22.1.0; else exec /run/current-system/sw/bin/node "$@"; fi'';
  fakeNixpkgs = pkgs.writeTextDir "flake.nix" ''
    {
      outputs = { self }:
        let
          mk = name: bin: script: derivation {
            inherit name;
            system = "x86_64-linux";
            builder = "${pkgs.bash}/bin/bash";
            args = [ "-c" "${pkgs.coreutils}/bin/mkdir -p $out/bin && ${pkgs.coreutils}/bin/cp ''${script} $out/bin/''${bin}" ];
          };
        in {
          legacyPackages.x86_64-linux = {
            hello = mk "hello-2.12" "hello" "${fakeBin "hello" "echo Hello from the stand-in nixpkgs"}";
            greeter = mk "greeter-1.0" "greet" "${fakeBin "greet" "echo greetings"}";
            nodejs_22 = mk "nodejs-22.1.0" "node" "${fakeNode}";
          };
        };
    }
  '';
  # nix-locate over fakeNixpkgs (the real index is another worker's part
  # of the base): greeter provides greet, so the name differs.
  fakeNixLocate = pkgs.writeShellScriptBin "nix-locate" ''
    # the flags the installer passes (nix-index 0.1.11 has no --top-level)
    [ "$*" = "--minimal --no-group --type x --type s --whole-name --at-root ''${!#}" ] || exit 2
    case "$*" in
      */bin/hello) echo hello.out ;;
      */bin/greet) echo gr.eet.out; echo zz-greet-extra.out; echo greeter.out ;;
      */bin/node) echo nodejs_22.out; echo nodejs_20.out ;;
    esac
  '';
  # The registry document for one packed tarball.
  npmMeta = pkgs.writers.writePython3 "npm-meta" { } ''
    import base64
    import hashlib
    import json
    import os
    import sys

    tgz = sys.argv[1]
    b = open(tgz, "rb").read()
    name = "fake-tool"
    url = "http://127.0.0.1:4874/" + os.path.basename(tgz)
    sri = "sha512-" + base64.b64encode(hashlib.sha512(b).digest()).decode()
    v = {
        "name": name,
        "version": "1.0.0",
        "bin": {"fake-tool": "cli.sh"},
        "dist": {
            "tarball": url,
            "shasum": hashlib.sha1(b).hexdigest(),
            "integrity": sri,
        },
    }
    doc = {
        "name": name,
        "dist-tags": {"latest": "1.0.0"},
        "versions": {"1.0.0": v},
    }
    print(json.dumps(doc))
  '';
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
          out = guest.succeed("ssh -n -F /dev/null -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes -o CertificateFile=/root/user-cert.pub -i /root/user dev@127.0.0.1 id")
          assert "uid=1000(dev)" in out, out
          guest.succeed("ssh-keygen -q -s /root/ca -I 'user:other' -n 0192e4b0-ffff-7000-8000-00000000beef -V -1m:+12h /root/user.pub")
          err = guest.fail("ssh -n -F /dev/null -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes -o CertificateFile=/root/user-cert.pub -i /root/user dev@127.0.0.1 id 2>&1")
          assert "Permission denied" in err, err
          guest.fail("ssh -n -F /dev/null -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes -i /root/user root@127.0.0.1 id")

      with subtest("store overlay and profile pinning"):
          mounts = guest.succeed("mount | grep -E 'ro-store|rw-store|/nix/store'")
          assert "overlay" in mounts, mounts
          # hello is in the host store (lower). Install it into dev's profile
          # and assert the pin copies its closure into the upper dir.
          hello = "${pkgs.hello}"
          guest.succeed(f"sudo -u dev nix profile install --offline {hello}")
          guest.succeed("systemctl start repose-pin-profile.service")
          upper = guest.succeed("awk '$2 == \"/nix/store\" { print $4 }' /proc/mounts | tr , '\\n' | grep '^upperdir=' | cut -d= -f2").strip()
          # the overlay appears twice (initrd and stage 2 views); one line is enough.
          # Mounted by stage 1, it names /sysroot (systemd initrd) or /mnt-root (scripted, I-231).
          upper = upper.splitlines()[0].removeprefix("/sysroot").removeprefix("/mnt-root")
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
    nodes.guest = { ... }: {
      imports = [ node ];
      environment.systemPackages = [ envProbeAgent ];
    };
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
          # I-249: the plain not-found line, then the two commands aligned.
          lines = [l for l in out.splitlines() if l.strip()]
          i = lines.index("cowsay: command not found")
          assert lines[i + 1] == "  nix profile add nixpkgs#cowsay  install it on this machine", out
          assert lines[i + 2] == "  repose config add cowsay        keep it on every rebuild (run this on your laptop)", out
          assert "is not installed" not in out, out
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

      # I-227: every package manager's user bin dir resolves from a login
      # shell, a non-login child of one, `sh -c` in a tmux window (how
      # `repose run` starts an agent), an interactive tmux pane, and a user
      # unit; and a base applied without reboot refreshes the running tmux
      # server and user manager.
      bin_dirs = ${builtins.toJSON userBinDirsForTest}
      probes = {}
      for i, d in enumerate(bin_dirs):
          name = f"repose-probe-{i}"
          probes[name] = d
          guest.succeed(f"install -d -o dev -g dev /home/dev/{d} && printf '#!/bin/sh\\necho ok\\n' > /home/dev/{d}/{name} && chmod 755 /home/dev/{d}/{name} && chown dev:dev /home/dev/{d}/{name}")
      names = " ".join(probes)
      guest.succeed(f"""cat > /tmp/check-path <<'EOF'
      #!/bin/sh
      for n in {names}; do
        command -v "$n" >/dev/null 2>&1 || echo "missing $n"
      done
      echo done
      EOF
      chmod 755 /tmp/check-path""")

      def check(where, out):
          out = out.strip()
          missing = [probes[l.split()[1]] for l in out.splitlines() if l.startswith("missing ")]
          assert out.endswith("done") and not missing, f"{where}: not on PATH: {missing}\n{out}"

      guest.succeed("install -d -o dev -g dev -m 0700 /home/dev/.repose")
      guest.succeed("""echo '{"project_id":"0192e4b0-0000-7000-8000-000000000001","slug":"todo-app","name":"todo-app","tz":"UTC","class":"large"}' > /home/dev/.repose/project.json && chown dev:dev /home/dev/.repose/project.json""")
      guest.wait_until_succeeds("sudo -H -u dev tmux ls | grep -q '^todo-app:'", timeout=60)

      # `repose run` reaches the guest over SSH and starts an agent with
      # `tmux new-window <cmd>` from that SSH command; tmux gives a window
      # started by a client outside tmux the client's PATH. So the agent
      # path is tested through a real sshd, as the CLI does it.
      guest.succeed("ssh-keygen -q -t ed25519 -N ''' -f /root/ca && ssh-keygen -q -t ed25519 -N ''' -f /root/user")
      guest.succeed("install -m 0644 /root/ca.pub /run/repose/user_ca.pub")
      guest.succeed("echo 0192e4b0-0000-7000-8000-000000000001 > /etc/ssh/principals/dev && systemctl reload sshd")
      guest.succeed("ssh-keygen -q -s /root/ca -I 'user:t' -n 0192e4b0-0000-7000-8000-000000000001 -V -1m:+12h /root/user.pub")

      def ssh(cmd):
          return guest.succeed("ssh -n -F /dev/null -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes -o CertificateFile=/root/user-cert.pub -i /root/user dev@127.0.0.1 " + shlex.quote(cmd))

      def wait_out(tag):
          return guest.wait_until_succeeds(f"grep -q done /tmp/out-{tag} && cat /tmp/out-{tag}", timeout=30)

      def via_tmux_command(tag):
          guest.succeed(f"rm -f /tmp/out-{tag}")
          ssh(f"tmux new-window -d -t todo-app 'sh -c /tmp/check-path > /tmp/out-{tag} 2>&1'")
          return wait_out(tag)

      # What runs with the tmux server's own environment: run-shell, #()
      # status jobs, and windows opened from inside tmux with a command.
      def via_tmux_server(tag):
          guest.succeed(f"rm -f /tmp/out-{tag}")
          guest.succeed(f"sudo -H -u dev timeout 30 tmux run-shell -t todo-app '/tmp/check-path > /tmp/out-{tag} 2>&1' < /dev/null")
          return wait_out(tag)

      def via_user_unit():
          return guest.succeed("sudo -H -u dev XDG_RUNTIME_DIR=/run/user/1000 systemd-run --user --quiet --wait --pipe /tmp/check-path")

      with subtest("I-227: user bin dirs on PATH everywhere"):
          check("bash -lc", dev("/tmp/check-path"))
          check("bash -c child of a login shell", dev("bash -c /tmp/check-path"))
          check("sh -c child of a login shell", dev("sh -c /tmp/check-path"))
          check("ssh command (non-login bash -c)", ssh("/tmp/check-path"))
          check("ssh command, sh -c child", ssh("sh -c /tmp/check-path"))
          check("tmux new-window sh -c over ssh (repose run)", via_tmux_command("cmd"))
          check("tmux server environment (run-shell)", via_tmux_server("server"))
          guest.succeed("rm -f /tmp/out-pane")
          # The session's own first window: the interactive login shell a
          # user attaches to.
          guest.succeed("sudo -H -u dev tmux send-keys -t todo-app:shell '/tmp/check-path > /tmp/out-pane 2>&1' Enter")
          check("tmux interactive pane", wait_out("pane"))
          check("systemd-run --user", via_user_unit())

      # I-241: an agent `repose run "prompt"` starts is tmux new-window's
      # command over SSH, a bash that is neither login nor interactive, in
      # a tmux server started by a user unit before the project's env and
      # any secret existed. The agent wrapper sources
      # /etc/profile.d/repose.sh, so it sees both. The probe is wrapped by
      # the same wrap.nix as the five agents and started the way
      # startAgentWindow starts one.
      with subtest("I-241: an agent started by tmux new-window over ssh sees REPOSE_PROJECT and named secrets"):
          guest.succeed("printf 'TZ=UTC\\nREPOSE_PROJECT=todo-app\\n' > /etc/repose/env")
          guest.succeed("install -m 0400 -o dev -g dev /dev/null /run/repose/secrets.env && echo \"export MY_TOKEN='s3cr3t'\" > /run/repose/secrets.env")
          guest.succeed("rm -f /tmp/out-raw /tmp/out-agent")
          # Without the wrapper (evidence only: what a bare command gets).
          ssh("tmux new-window -t todo-app -n raw -c ~/todo-app -d 'sh -c \"echo project=$REPOSE_PROJECT secret=$MY_TOKEN; echo done\" > /tmp/out-raw 2>&1'")
          print("bare new-window command: " + wait_out("raw"))
          ssh("tmux new-window -t todo-app -n probe -c ~/todo-app -d 'repose-env-probe'")
          out = wait_out("agent")
          print(out)
          assert "project=todo-app" in out, out
          assert "secret=s3cr3t" in out, out
          assert "path=ok" in out, out
          assert "agent=repose-env-probe" in out, out

      with subtest("I-227: installs land on PATH (go install, npm i -g)"):
          guest.succeed("""install -d -o dev -g dev /tmp/gi /tmp/npmpkg/bin && cat > /tmp/gi/go.mod <<'EOF'
      module example.com/gi

      go 1.22
      EOF
      cat > /tmp/gi/main.go <<'EOF'
      package main

      import "fmt"

      func main() { fmt.Println("gi ok") }
      EOF
      cat > /tmp/npmpkg/package.json <<'EOF'
      {"name": "repose-npm-probe", "version": "1.0.0", "bin": {"repose-npm-probe": "bin/probe.js"}}
      EOF
      printf '#!/usr/bin/env node\\nconsole.log("npm ok")\\n' > /tmp/npmpkg/bin/probe.js
      chmod 755 /tmp/npmpkg/bin/probe.js
      chown -R dev:dev /tmp/gi /tmp/npmpkg""")
          dev("cd /tmp/gi && GOTOOLCHAIN=local GOFLAGS=-mod=mod GOCACHE=/tmp/gi/cache go install .")
          assert dev("gi").strip() == "gi ok"
          dev("npm i -g --offline /tmp/npmpkg")
          assert dev("repose-npm-probe").strip() == "npm ok"
          assert guest.succeed("sudo -H -u dev XDG_RUNTIME_DIR=/run/user/1000 systemd-run --user --quiet --wait --pipe sh -c gi").strip() == "gi ok"

      with subtest("I-227: activation refreshes a running tmux server and user manager"):
          guest.succeed("sudo -H -u dev tmux set-environment -g PATH /run/current-system/sw/bin")
          guest.succeed("sudo -H -u dev XDG_RUNTIME_DIR=/run/user/1000 systemctl --user set-environment PATH=/run/current-system/sw/bin")
          out = via_tmux_server("stale")
          assert "missing" in out, out
          out = via_user_unit()
          assert "missing" in out, out
          guest.succeed("/run/current-system/activate")
          check("tmux server environment after activation", via_tmux_server("fresh"))
          check("systemd-run --user after activation", via_user_unit())
    '';
  };

  # I-221, I-222 on a real base: the CLI's tools part (the golden list from
  # internal/cli's TestToolsGuestPartsGolden) plans, the user unit installs
  # in the background, and every tool resolves in a new login shell.
  # Offline: "nixpkgs" is a stand-in flake (fakeNixpkgs) whose packages
  # build from what the guest's store already has, nix-locate is a stand-in
  # index over it, and npm installs from a registry on 127.0.0.1. go,
  # cargo and uv installs need the network and are not exercised here.
  guest-tools-carry = mkTest "guest-tools-carry" {
    nodes.guest = { lib, ... }: {
      imports = [ node ];
      environment.systemPackages = [ pkgs.python3 fakeNixLocate ];
      # The stand-in index, not the base's real one (I-219), answers
      # nix-locate here: the real index knows nothing of the fake tools.
      repose.nixIndexPackage = lib.mkForce null;
      virtualisation.additionalPaths = [ fakeNixpkgs pkgs.bash pkgs.coreutils ];
      nix.registry.nixpkgs.to = lib.mkForce { type = "path"; path = "${fakeNixpkgs}"; };
      nix.settings.flake-registry = lib.mkForce "";
      # The stand-in packages name their builder's paths as plain strings.
      nix.settings.sandbox = lib.mkForce false;
      nix.settings.substituters = lib.mkForce [ ];
      # The store overlay's upper on the disk, as on a real guest's thin
      # volume, so what dev installed survives the reboot subtest.
      virtualisation.writableStoreUseTmpfs = false;
    };
    testScript = ''
      import json
      guest.start()
      guest.wait_for_unit("multi-user.target")
      guest.wait_for_unit("user@1000.service")
      guest.succeed("install -d -o dev -g dev -m 0700 /home/dev/.repose")
      guest.succeed("mkdir -p /tmp/p && cp -r ${guestParts}/. /tmp/p && chmod -R u+w /tmp/p && chown -R dev:dev /tmp/p")
      wanted = json.loads(guest.succeed("cat /tmp/p/tools/wanted.json"))

      # The npm registry: fake-tool@1.0.0, packed here, served by python.
      guest.succeed("mkdir -p /tmp/fake-tool /srv/npm")
      guest.succeed("""printf '%s\n' '{"name":"fake-tool","version":"1.0.0","bin":{"fake-tool":"cli.sh"}}' > /tmp/fake-tool/package.json""")
      guest.succeed("printf '#!/bin/sh\\necho fake-tool ok\\n' > /tmp/fake-tool/cli.sh && chmod +x /tmp/fake-tool/cli.sh")
      guest.succeed("cd /srv/npm && HOME=/tmp npm pack /tmp/fake-tool")
      guest.succeed("${npmMeta} /srv/npm/fake-tool-1.0.0.tgz > /srv/npm/fake-tool")
      guest.succeed("systemd-run --unit fake-npm python3 -m http.server 4874 --bind 127.0.0.1 --directory /srv/npm")
      guest.wait_for_open_port(4874, "127.0.0.1")
      guest.succeed("sudo -H -u dev sh -c 'echo registry=http://127.0.0.1:4874/ > /home/dev/.npmrc'")
      assert guest.succeed("sudo -H -u dev bash -lc 'node --version'").strip().startswith("v24."), "the base's node is not 24"

      with subtest("plan: the one line, the installing file, no install yet"):
          out = guest.succeed("sudo -H -u dev XDG_RUNTIME_DIR=/run/user/1000 sh -e /tmp/p/tools.sh /tmp/p")
          print(out)
          assert "#installing fake-tool greet hello nonexistent-cmd nodejs_22" in out, out
          listed = guest.succeed("cat /run/user/1000/repose-installing").split()
          assert sorted(listed) == ["fake-tool", "greet", "hello", "nonexistent-cmd"], listed
          assert guest.succeed("cat /home/dev/.repose/tools-wanted.json").strip() == json.dumps(wanted, separators=(",", ":"))

      with subtest("run: every tool resolves in a new login shell, the installing file is gone"):
          guest.wait_until_succeeds(f"grep -qx {wanted['hash']} /home/dev/.repose/carry/tools", timeout=900)
          print(guest.succeed("cat /home/dev/.repose/tools-install.log"))
          assert "stand-in" in guest.succeed("sudo -H -u dev bash -lc 'hello'")
          assert guest.succeed("sudo -H -u dev bash -lc 'greet'").strip() == "greetings"
          assert guest.succeed("sudo -H -u dev bash -lc 'fake-tool'").strip() == "fake-tool ok"
          assert guest.succeed("sudo -H -u dev bash -lc 'node --version'").strip() == "v22.1.0"
          guest.fail("test -e /run/user/1000/repose-installing")
          ilog = guest.succeed("cat /home/dev/.repose/tools-install.log")
          assert "hello: installed nixpkgs#hello" in ilog, ilog
          assert "greet: installed nixpkgs#greeter" in ilog, ilog
          assert "fake-tool: installed with npm" in ilog, ilog
          assert "node: nodejs_22 is the node of new shells" in ilog, ilog
          # dev's profile holds them, so the store overlay pins them
          profile = guest.succeed("sudo -H -u dev nix profile list")
          assert "hello" in profile and "greeter" in profile and "nodejs_22" in profile, profile

      with subtest("what could not be installed is said once"):
          notices = guest.succeed("cat /home/dev/.repose/tools-notices")
          assert "Could not install nonexistent-cmd: no nixpkgs package has bin/nonexistent-cmd" in notices, notices
          out = guest.succeed("sudo -H -u dev sh -e /tmp/p/tools-notices.sh /tmp/p")
          assert "#warn Could not install nonexistent-cmd" in out, out
          assert guest.succeed("sudo -H -u dev sh -e /tmp/p/tools-notices.sh /tmp/p").strip() == ""

      with subtest("the same list again installs nothing and says nothing"):
          out = guest.succeed("sudo -H -u dev XDG_RUNTIME_DIR=/run/user/1000 sh -e /tmp/p/tools.sh /tmp/p")
          assert "#installing" not in out and "#warn" not in out, out
          guest.fail("test -e /run/user/1000/repose-installing")

      with subtest("a pass cut short is finished by the unit at the next boot"):
          guest.succeed("rm /home/dev/.repose/carry/tools")
          guest.succeed("sudo -H -u dev nix profile remove greeter")
          guest.fail("sudo -H -u dev bash -lc 'command -v greet'")
          guest.shutdown()
          guest.start()
          guest.wait_for_unit("user@1000.service")
          guest.wait_until_succeeds(f"grep -qx {wanted['hash']} /home/dev/.repose/carry/tools", timeout=900)
          print(guest.succeed("tail -n 12 /home/dev/.repose/tools-install.log"))
          assert guest.succeed("sudo -H -u dev bash -lc 'greet'").strip() == "greetings"
    '';
  };

  guest-compat = mkTest "guest-compat" {
    nodes.guest = node;
    testScript = ''
      guest.start()
      guest.wait_for_unit("multi-user.target")

      import shlex

      def dev(cmd):
          return guest.succeed(f"sudo -H -u dev bash -lc {shlex.quote(cmd)}")

      def fetch(path, method="GET"):
          return guest.succeed(f"curl -s --path-as-is -X {method} -o /dev/null -w '%{{http_code}} %{{redirect_url}}' 'http://127.0.0.1:850{path}'").strip()

      with subtest("I-228: Prisma's mirror redirects linux-nixos to the debian build"):
          guest.wait_for_unit("repose-prisma-engines.socket")
          mirror = dev("echo -n $PRISMA_ENGINES_MIRROR")
          assert mirror == "http://127.0.0.1:850", mirror
          up = "https://binaries.prisma.sh/all_commits/${prismaCommit}"
          out = fetch("/all_commits/${prismaCommit}/linux-nixos/schema-engine.gz")
          assert out == f"302 {up}/debian-openssl-3.0.x/schema-engine.gz", out
          out = fetch("/all_commits/${prismaCommit}/linux-nixos/libquery_engine.so.node.gz.sha256")
          assert out == f"302 {up}/debian-openssl-3.0.x/libquery_engine.so.node.gz.sha256", out
          # Any other target (a deploy target in binaryTargets) is untouched.
          out = fetch("/all_commits/${prismaCommit}/rhel-openssl-3.0.x/libquery_engine.so.node.gz")
          assert out == f"302 {up}/rhel-openssl-3.0.x/libquery_engine.so.node.gz", out
          assert fetch("/all_commits/x/linux-nixos/a", "HEAD").startswith("302 "), "HEAD"
          assert fetch("/all_commits/x/linux-nixos/a", "POST").startswith("405"), "POST"
          assert fetch("/a/../etc/passwd").startswith("400"), "dotdot"
          assert fetch("/a%20b").startswith("400"), "escape"
          # Never auto-forwarded: loopback, under 1024 (I-199).
          guest.succeed("ss -Hltn | grep -q '127.0.0.1:850 '")

      with subtest("I-228: the debian engine the redirect names runs through nix-ld"):
          guest.succeed("install -d -o dev -g dev /tmp/prisma")
          guest.succeed("gzip -dc ${prismaSchemaEngineGz} > /tmp/prisma/schema-engine-linux-nixos && chmod +x /tmp/prisma/schema-engine-linux-nixos && chown dev:dev /tmp/prisma/schema-engine-linux-nixos")
          out = dev("/tmp/prisma/schema-engine-linux-nixos --version")
          assert out.strip() == "schema-engine-cli ${prismaCommit}", out

      with subtest("I-228: Playwright's directory is writable and seeded"):
          d = "/home/dev/.cache/ms-playwright"
          path = dev("echo -n $PLAYWRIGHT_BROWSERS_PATH")
          assert path == d, path
          guest.wait_until_succeeds("systemctl show -p Result repose-playwright-seed.service | grep -q success && test -L /home/dev/.cache/ms-playwright/chromium_headless_shell-*")
          names = guest.succeed("ls ${pkgs.reposePlaywrightBrowsers}").split()
          chromium = [n for n in names if n.startswith("chromium-")][0]
          for n in names:
              dev(f"test -L {d}/{n} && test -d {d}/{n}/")
          dev(f"touch {d}/probe && rm {d}/probe")
          # A user's own download is left alone, a stale store link goes,
          # a missing link comes back.
          dev(f"rm {d}/{chromium} && mkdir {d}/{chromium} && ln -s /nix/store/00000000000000000000000000000000-gone {d}/chromium-1 && rm {d}/ffmpeg-*")
          guest.succeed("systemctl restart repose-playwright-seed.service")
          dev(f"test -d {d}/{chromium} && ! test -L {d}/{chromium}")
          dev(f"! test -e {d}/chromium-1 && ! test -L {d}/chromium-1")
          dev(f"test -L {d}/ffmpeg-* || ! ls ${pkgs.reposePlaywrightBrowsers} | grep -q ffmpeg")
          guest.succeed("systemctl restart repose-playwright-seed.service")
          # The MCP server still reads the store path from its own wrapper.
          guest.succeed("grep -q 'PLAYWRIGHT_BROWSERS_PATH=.*${pkgs.reposePlaywrightBrowsers}' ${pkgs.reposeMcp.playwright-mcp}/bin/playwright-mcp")

      with subtest("I-228: a manylinux wheel imports under the system python and its venvs"):
          guest.succeed("install -d -o dev -g dev /tmp/py && cp ${numpyWheel} /tmp/py/${numpyWheelName} && chown dev:dev /tmp/py/*")
          dev("cd /tmp/py && python3 -m venv v && v/bin/pip install -q --no-index /tmp/py/${numpyWheelName}")
          out = dev("env -u LD_LIBRARY_PATH /tmp/py/v/bin/python -c 'import numpy, sys; print(numpy.__version__, sys.prefix)'")
          assert out.strip() == "2.3.3 /tmp/py/v", out
          out = dev("cd /tmp/py && . v/bin/activate && python -c 'import numpy; print(numpy.ones(3).sum())'")
          assert out.strip() == "3.0", out
          dev("cd /tmp/py && UV_OFFLINE=1 uv venv -q -p python3 u && UV_OFFLINE=1 VIRTUAL_ENV=/tmp/py/u uv pip install -q --no-index /tmp/py/${numpyWheelName}")
          out = dev("env -u LD_LIBRARY_PATH /tmp/py/u/bin/python -c 'import numpy; print(numpy.__version__)'")
          assert out.strip() == "2.3.3", out
          out = dev("python3 -c 'import ssl, sqlite3, sys; print(sys.executable)'")
          assert out.strip() == "/run/current-system/sw/bin/python3", out

      with subtest("I-228: pkg-config finds the common system libraries"):
          out = dev("pkg-config --modversion openssl zlib sqlite3 libffi")
          assert len(out.split()) == 4, out
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
          # playwright's browsers are the packaged ones, linked into the
          # writable directory at boot (I-228), not a download.
          guest.succeed("sudo -u dev bash -lc 'test -d \"$PLAYWRIGHT_BROWSERS_PATH\" && ls \"$PLAYWRIGHT_BROWSERS_PATH\" | grep -q chromium'")
    '';
  };

  # I-243: the machine guide is installed where each of the five agents
  # reads global instructions, each agent really sends it to its model
  # (a stand-in API records the first request), the user's own instruction
  # files are left byte for byte, and every command the guide names is on
  # the machine. A stand-in repose-notify shows a `needs:` line rendered
  # when its command exists; one whose command is missing is dropped.
  guest-agent-guide = mkTest "guest-agent-guide" {
    nodes.guest = { ... }: {
      imports = [ node ];
      environment.systemPackages = [ (pkgs.writeShellScriptBin "repose-notify" "exit 0") ];
    };
    testScript = ''
      import json
      import re
      import shlex
      import tomllib

      guest.start()
      guest.wait_for_unit("multi-user.target")

      def dev(cmd, env=""):
          return guest.succeed(f"sudo -H -u dev {env} bash -lc {shlex.quote(cmd)}")

      def has(cmd):
          return guest.execute(f"sudo -H -u dev bash -lc {shlex.quote('command -v ' + cmd)}")[0] == 0

      source = open("${../base/agent-guide.md}").read()
      commands = [l.split() for l in open("${../base/agent-guide.commands}") if l.strip() and not l.startswith("#")]

      with subtest("rendered from the source: comments dropped, needs lines follow the guest"):
          guide = guest.succeed("cat /etc/repose/agent-guide.md")
          out, in_comment = [], False
          for line in source.splitlines():
              if in_comment:
                  in_comment = "-->" not in line
                  continue
              if line.startswith("<!--") and "-->" not in line:
                  in_comment = True
                  continue
              m = re.search(r"<!-- needs: ([A-Za-z0-9._-]+) -->", line)
              if m and not has(m.group(1)):
                  continue
              out.append(re.sub(r"\s*<!--.*?-->", "", line).rstrip())
          while out and out[0] == "":
              out.pop(0)
          assert guide == "\n".join(out) + "\n", guide
          assert "<!--" not in guide, guide
          assert "`repose-notify " in guide, "a needs line whose command exists is rendered"
          for c in [c[0] for c in commands if c[1:] == ["needs"] and not has(c[0])]:
              assert f"`{c} " not in guide, f"{c} is missing but the guide tells agents to run it"

      with subtest("installed where each agent reads it"):
          assert guest.succeed("cat /etc/claude-code/CLAUDE.md") == guide
          assert guest.succeed("cat /etc/repose/gemini-extension/GEMINI.md") == guide
          codex = tomllib.loads(guest.succeed("cat /etc/codex/config.toml"))
          assert codex == {"developer_instructions": guide}, codex
          oc = json.loads(guest.succeed("cat /etc/opencode/opencode.json"))
          assert oc["instructions"] == ["/etc/repose/agent-guide.md"], oc
          ext = json.loads(guest.succeed("cat /etc/repose/gemini-extension/gemini-extension.json"))
          assert ext["contextFileName"] == "GEMINI.md", ext
          guest.succeed("grep -q /etc/repose/agent-guide.md /etc/repose/pi-extension.js")

      with subtest("every command the guide names is on the machine"):
          for c in commands:
              if c[1:] != ["needs"]:
                  dev(f"command -v {c[0]}")

      with subtest("each agent sends the guide and the user's own instructions"):
          guest.succeed("systemd-run --unit capture-llm ${pkgs.python3}/bin/python3 ${./capture-llm.py} 18777 /tmp/caps")
          guest.wait_until_succeeds("curl -s -o /dev/null http://127.0.0.1:18777/")
          files = {
              ".claude/CLAUDE.md": "USER-CLAUDE-MARK",
              ".codex/AGENTS.md": "USER-CODEX-MARK",
              ".config/opencode/AGENTS.md": "USER-OPENCODE-MARK",
              ".gemini/GEMINI.md": "USER-GEMINI-MARK",
              ".pi/agent/AGENTS.md": "USER-PI-MARK",
          }
          for f, mark in files.items():
              dev(f"mkdir -p $(dirname ~/{f}) && printf '# mine\\n{mark}\\n' > ~/{f}")
          dev("""printf 'model_provider = "fake"\\n[model_providers.fake]\\nname = "fake"\\nbase_url = "http://127.0.0.1:18777/v1"\\nenv_key = "FAKE_KEY"\\nwire_api = "responses"\\n' > ~/.codex/config.toml""")
          dev("""echo '{"provider":{"fake":{"npm":"@ai-sdk/openai-compatible","options":{"baseURL":"http://127.0.0.1:18777/v1","apiKey":"x"},"models":{"m":{}}}},"model":"fake/m"}' > ~/.config/opencode/opencode.json""")
          dev("""echo '{"security":{"auth":{"selectedType":"gemini-api-key"}}}' > ~/.gemini/settings.json""")
          dev("""echo '{"providers":{"fake":{"baseUrl":"http://127.0.0.1:18777/v1","api":"openai-completions","apiKey":"x","models":[{"id":"m"}]}}}' > ~/.pi/agent/models.json""")
          dev("mkdir -p ~/proj")
          before = dev("cd ~ && sha256sum " + " ".join(files))
          runs = {
              "claude": ("USER-CLAUDE-MARK", "ANTHROPIC_BASE_URL=http://127.0.0.1:18777 ANTHROPIC_API_KEY=sk-x CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "claude -p hi --max-turns 1"),
              "codex": ("USER-CODEX-MARK", "FAKE_KEY=x", "codex exec --skip-git-repo-check hi"),
              "opencode": ("USER-OPENCODE-MARK", "OPENCODE_DISABLE_MODELS_FETCH=1 OPENCODE_DISABLE_AUTOUPDATE=1", "opencode run hi"),
              "gemini": ("USER-GEMINI-MARK", "GEMINI_CLI_TRUST_WORKSPACE=true GEMINI_API_KEY=x GOOGLE_GEMINI_BASE_URL=http://127.0.0.1:18777", "gemini -p hi"),
              "pi": ("USER-PI-MARK", "PI_OFFLINE=1", "pi --provider fake --model m -p hi"),
          }
          sentinel = "This is a repose machine"
          for agent, (mark, env, cmd) in runs.items():
              guest.succeed("rm -rf /tmp/caps/*")
              print(guest.execute(f"sudo -H -u dev {env} bash -lc {shlex.quote('cd ~/proj && timeout 180 ' + cmd)} 2>&1 | tail -5")[1])
              bodies = guest.succeed("cat /tmp/caps/* 2>/dev/null || true")
              assert sentinel in bodies, f"{agent}: the guide is not in what it sent: {bodies[:3000]}"
              assert mark in bodies, f"{agent}: the user's own instructions are not in what it sent"
          after = dev("cd ~ && sha256sum " + " ".join(files))
          assert before == after, (before, after)

      with subtest("gemini and pi links: idempotent, a user's file at the path is left alone"):
          assert dev("readlink ~/.gemini/extensions/repose-machine-guide").strip() == "/etc/repose/gemini-extension"
          assert dev("readlink ~/.pi/agent/extensions/repose-machine-guide.js").strip() == "/etc/repose/pi-extension.js"
          dev("mkdir -p ~/.gemini/extensions/mine && echo '{}' > ~/.gemini/extensions/mine/gemini-extension.json")
          listing = dev("ls -la ~/.gemini/extensions ~/.pi/agent/extensions")
          for _ in range(2):
              dev("repose-agent-setup gemini && repose-agent-setup pi")
          assert dev("ls -la ~/.gemini/extensions ~/.pi/agent/extensions") == listing
          # a stale link of ours is repointed
          dev("ln -sfn /nonexistent ~/.pi/agent/extensions/repose-machine-guide.js && repose-agent-setup pi")
          assert dev("readlink ~/.pi/agent/extensions/repose-machine-guide.js").strip() == "/etc/repose/pi-extension.js"
          # something that is not a link stays
          dev("mkdir -p /tmp/h2/.pi/agent/extensions && echo mine > /tmp/h2/.pi/agent/extensions/repose-machine-guide.js")
          dev("HOME=/tmp/h2 repose-agent-setup pi")
          assert dev("cat /tmp/h2/.pi/agent/extensions/repose-machine-guide.js").strip() == "mine"
    '';
  };
}
