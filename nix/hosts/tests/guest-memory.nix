# host-guest-memory (DECISIONS I-230): a real Cloud Hypervisor guest in a
# memory cgroup shaped the way hostd shapes guest@<id>, keeping most of its
# RAM in use and writing gigabytes to its disk, run once with the unit
# shape and --disk options hostd rendered before I-230 and once with what
# it renders now.
#
# The incident it reproduces: on host-01 a large guest doing heavy installs
# was OOM-killed by its own unit's memory cgroup. The guest's RAM is a
# shared memfd charged to the unit as shmem; the hypervisor wrote the disk
# through the host page cache (no O_DIRECT), those pages were charged to
# the same unit, and pages under writeback cannot be reclaimed on demand,
# so the 512 MiB overhead filled and the kernel killed cloud-hypervisor.
#
# This is a script, not a NixOS VM test: inside a test VM the guest is a
# third level of virtualisation, and on the dev box (itself an Azure VM)
# it never printed a line in five minutes. The script runs Cloud
# Hypervisor directly under `systemd-run --scope` (as the user, or as root
# on a host), which gives it a real memory cgroup with the same limits:
#
#   nix build ./nix#checks.x86_64-linux.host-guest-memory
#   ./result/bin/repose-guest-memory-repro DIR     # DIR on a real disk, 2 GiB free
#
# The unit shape and the disk option are read from hostd's goldens
# (internal/hostd/guest/testdata/unit.golden for the large class,
# internal/hostd/ch/testdata/args.golden for the --disk value); only the
# class is shrunk to 512 MiB.
{ pkgs }:
let
  lib = pkgs.lib;
  unitGolden = lib.splitString "\n" (builtins.readFile ../../../internal/hostd/guest/testdata/unit.golden);
  argsGolden = lib.splitString "\n" (builtins.readFile ../../../internal/hostd/ch/testdata/args.golden);
  mib = prefix: lib.toInt (lib.removeSuffix "M" (lib.removePrefix prefix (lib.findFirst (l: lib.hasPrefix prefix l) "" unitGolden)));
  largeMiB = 8192;
  overheadMiB = mib "MemoryMax=" - largeMiB;
  highMarginMiB = mib "MemoryMax=" - mib "MemoryHigh=";
  diskLine = lib.findFirst (l: lib.hasPrefix "path=" l) "" argsGolden;
  diskOpts = lib.concatStringsSep "," (lib.tail (lib.splitString "," diskLine));

  guestMiB = 512;
  kernel = pkgs.linuxPackages.kernel;
  modules = pkgs.makeModulesClosure {
    # The modules are the kernel's separate output.
    kernel = kernel.modules;
    firmware = kernel.modules;
    rootModules = [ "virtio_pci" "virtio_blk" "ext4" ];
  };
  guestInit = pkgs.writeScript "guest-init" ''
    #!${pkgs.busybox}/bin/sh
    export PATH=${lib.makeBinPath [ pkgs.busybox pkgs.kmod pkgs.e2fsprogs ]}
    mount -t proc proc /proc
    mount -t sysfs sys /sys
    mount -t devtmpfs dev /dev
    # kmod's modprobe: busybox's has no -d and cannot read .ko.xz.
    ${pkgs.kmod}/bin/modprobe -d ${modules} virtio_pci
    ${pkgs.kmod}/bin/modprobe -d ${modules} virtio_blk
    ${pkgs.kmod}/bin/modprobe -d ${modules} ext4 || true
    mkdir -p /mnt
    for _ in $(seq 100); do [ -b /dev/vda ] && break; sleep 0.1; done
    if [ ! -b /dev/vda ]; then echo "GUEST-FAILED no /dev/vda"; poweroff -f; fi
    mode=$(sed -n 's/.*repose_test=\([a-z]*\).*/\1/p' /proc/cmdline)
    echo "GUEST lbs=$(cat /sys/block/vda/queue/logical_block_size) pbs=$(cat /sys/block/vda/queue/physical_block_size) mode=$mode"
    # Most of the guest's RAM in use, as with an agent, a dev server and a
    # language server running: 200 of 512 MiB pinned in a tmpfs, next to
    # the unpacked initramfs (about 70 MiB).
    mkdir -p /fill && mount -t tmpfs -o size=210m tmpfs /fill
    dd if=/dev/zero of=/fill/ram bs=1M count=200 2>/dev/null
    free -m | sed 's/^/GUEST /'
    now() { cut -d' ' -f1 /proc/uptime; }
    rate() { awk -v m="$1" -v a="$2" -v b="$3" 'BEGIN { printf "%.1f MiB/s (%.1f s)", m / (b - a), b - a }'; }
    tree=$(du -sm /nix/store | cut -f1)
    echo "GUEST small-file tree $tree MiB, $(find /nix/store | wc -l) entries"
    case "$mode" in
      throughput)
        t0=$(now); dd if=/dev/zero of=/dev/vda bs=1M count=1024 oflag=direct 2>/dev/null; t1=$(now)
        echo "RESULT seq-write-direct-1M $(rate 1024 "$t0" "$t1")"
        t0=$(now); dd if=/dev/zero of=/dev/vda bs=1M count=1024 conv=fsync 2>/dev/null; t1=$(now)
        echo "RESULT seq-write-buffered-fsync $(rate 1024 "$t0" "$t1")"
        echo 3 > /proc/sys/vm/drop_caches
        t0=$(now); dd if=/dev/vda of=/dev/null bs=1M count=1024 iflag=direct 2>/dev/null; t1=$(now)
        echo "RESULT seq-read-direct-1M $(rate 1024 "$t0" "$t1")"
        t0=$(now); dd if=/dev/zero of=/dev/vda bs=4k count=8192 oflag=direct 2>/dev/null; t1=$(now)
        echo "RESULT write-direct-4k-qd1 $(awk -v a="$t0" -v b="$t1" 'BEGIN { printf "%.0f IOPS", 8192 / (b - a) }')"
        { mkfs.ext4 -q -F /dev/vda >/dev/null 2>&1 && mount /dev/vda /mnt; } || { echo "GUEST-FAILED mount"; poweroff -f; }
        t0=$(now)
        for i in 1 2 3 4 5; do cp -a /nix/store "/mnt/small-$i"; done
        sync; t1=$(now)
        echo "RESULT small-files-x5 $(rate $((tree * 5)) "$t0" "$t1")"
        umount /mnt
        ;;
      sustained)
        # The e2e-tools pattern: installs unpacking many small files and
        # some large ones (wheels, browsers, SDK tarballs), gigabytes in
        # minutes, no single RAM hog.
        { mkfs.ext4 -q -F /dev/vda >/dev/null 2>&1 && mount /dev/vda /mnt; } || { echo "GUEST-FAILED mount"; poweroff -f; }
        t0=$(now)
        for pass in 1 2 3 4 5 6; do
          cp -a /nix/store "/mnt/small-$pass"
          dd if=/dev/zero of="/mnt/big-$pass" bs=1M count=400 2>/dev/null
          rm -rf "/mnt/small-$((pass - 2))" "/mnt/big-$((pass - 2))"
          echo "GUEST pass $pass written"
        done
        sync; t1=$(now)
        echo "RESULT sustained $(rate $(((tree + 400) * 6)) "$t0" "$t1")"
        umount /mnt
        ;;
    esac
    echo GUEST-DONE
    sync
    poweroff -f
  '';
  initrd = pkgs.makeInitrd {
    contents = [{ object = guestInit; symlink = "/init"; }];
  };
in
pkgs.writeShellApplication {
  name = "repose-guest-memory-repro";
  runtimeInputs = [ pkgs.coreutils pkgs.gawk pkgs.gnugrep pkgs.gnused pkgs.systemd ];
  text = ''
    dir=''${1:?usage: repose-guest-memory-repro DIR (a real disk, 2 GiB free)}
    mkdir -p "$dir"
    scope=(--user)
    [ "$(id -u)" = 0 ] && scope=()
    fail=0 last_file=0 last_events="" last_done=0

    # run NAME DISK_OPTS MODE PROP...: boot the guest in a scope with PROPs,
    # sample its memory cgroup until it exits, print peaks and the guest's
    # RESULT lines. "nonshmem_file" is page cache other than the guest's
    # RAM: what the host cached for the hypervisor.
    run() {
      local name=$1 opts=$2 mode=$3; shift 3
      local props=() p
      for p in "$@"; do props+=(-p "$p"); done
      local disk="$dir/disk.img" log="$dir/$name.serial"
      [ -e "$disk" ] || truncate -s 2G "$disk"
      : > "$log"
      systemd-run "''${scope[@]}" --scope --quiet --unit "repose-gm-$name-$$" "''${props[@]}" -- \
        timeout 600 ${pkgs.cloud-hypervisor}/bin/cloud-hypervisor \
        --kernel ${kernel}/bzImage --initramfs ${initrd}/initrd \
        --cmdline "console=ttyS0 panic=-1 repose_test=$mode" \
        --cpus boot=2 --memory size=${toString guestMiB}M,shared=on \
        --disk "path=$disk,$opts" --serial "file=$log" --console off >/dev/null 2>&1 &
      local pid=$! cg="" rel
      for _ in $(seq 100); do
        # systemd-run execs the command once the scope exists; until then
        # the pid is still in the caller's cgroup.
        rel=$(grep -o '/.*repose-gm-.*\.scope' "/proc/$pid/cgroup" 2>/dev/null || true)
        if [ -n "$rel" ] && [ -r "/sys/fs/cgroup$rel/memory.stat" ]; then cg=/sys/fs/cgroup$rel; break; fi
        sleep 0.05
      done
      if [ -z "$cg" ]; then echo "no memory cgroup for $name"; wait "$pid" || true; fail=1; return; fi
      local pf=0 pw=0 pc=0 po=0 ev="" file shmem file_writeback file_dirty anon kernel cur e
      while kill -0 "$pid" 2>/dev/null; do
        file=0 shmem=0 file_writeback=0 file_dirty=0 anon=0 kernel=0
        eval "$(awk '/^(file|shmem|file_writeback|file_dirty|anon|kernel) / { print $1 "=" $2 }' "$cg/memory.stat" 2>/dev/null)"
        cur=$(cat "$cg/memory.current" 2>/dev/null || echo 0)
        if (( file - shmem > pf )); then pf=$((file - shmem)); fi
        if (( file_writeback + file_dirty > pw )); then pw=$((file_writeback + file_dirty)); fi
        if (( cur > pc )); then pc=$cur; fi
        if (( anon + kernel > po )); then po=$((anon + kernel)); fi
        if e=$(tr '\n' ' ' < "$cg/memory.events" 2>/dev/null) && [ -n "$e" ]; then ev=$e; fi
        sleep 0.1
      done
      wait "$pid" || true
      echo "== $name: disk $opts, $* (mode $mode)"
      echo "   peak_nonshmem_file_mib=$((pf >> 20)) peak_dirty_writeback_mib=$((pw >> 20)) peak_anon_kernel_mib=$((po >> 20)) peak_current_mib=$((pc >> 20))"
      echo "   memory.events: $ev"
      grep -aE '^(GUEST-|RESULT|GUEST lbs)' "$log" | sed 's/^/   /' || true
      last_file=$((pf >> 20)) last_events=$ev last_done=0
      if grep -aq GUEST-DONE "$log"; then last_done=1; fi
    }

    now=(MemoryMax=${toString (guestMiB + overheadMiB)}M MemoryHigh=${toString (guestMiB + overheadMiB - highMarginMiB)}M)
    before=(MemoryMax=${toString (guestMiB + 512)}M)

    for mode in throughput sustained; do
      run "$mode-before" image_type=raw "$mode" "''${before[@]}"
      # Before I-230: host page cache fills the overhead (or the kernel
      # kills the hypervisor, as on host-01).
      if (( last_file < 200 )) && ! grep -q 'oom_kill [1-9]' <<<"$last_events"; then
        echo "   UNEXPECTED: before did not fill the overhead"; fail=1
      fi
      run "$mode-after" "${diskOpts}" "$mode" "''${now[@]}"
      if (( last_file >= 64 )) || (( last_done == 0 )) || ! grep -q ' oom 0 oom_kill 0' <<<"$last_events"; then
        echo "   FAILED: after left host page cache in the unit or did not finish"; fail=1
      fi
    done
    rm -f "$dir/disk.img"
    exit "$fail"
  '';
}
