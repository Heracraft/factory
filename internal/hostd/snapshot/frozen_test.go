package snapshot

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heracraft/repose/internal/hostd/shell"
)

// mountedCopy mounts img on a loop device, runs write inside the mount,
// then copies the device the way hostd takes its LVM snapshot of a
// running guest: under fsfreeze when freeze is true, straight from the
// live mount otherwise. The copy is sparse, like a thin snapshot.
func mountedCopy(t *testing.T, img string, freeze bool, write func(mnt string) (release func())) string {
	t.Helper()
	mnt := filepath.Join(t.TempDir(), "mnt")
	if err := os.Mkdir(mnt, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(name string, args ...string) {
		t.Helper()
		if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v %s", name, args, err, out)
		}
	}
	run("mount", "-o", "loop", img, mnt)
	defer run("umount", mnt)
	if release := write(mnt); release != nil {
		defer release() // before the umount
	}
	snap := filepath.Join(t.TempDir(), "snap")
	if freeze {
		run("fsfreeze", "-f", mnt)
		defer run("fsfreeze", "-u", mnt)
	}
	run("cp", "--sparse=always", img, snap)
	return snap
}

// TestFrozenRunningVolumeRoundTrip is the running-guest case of I-164 on a
// real mounted ext4 (I-171): writes still in the page cache (delayed
// allocation), a deleted file, a file held open after unlink, then
// fsfreeze and a copy of the device. The frozen copy goes out as extents;
// every byte its block bitmaps mark used comes back identical, every
// other byte comes back zero, e2fsck passes and the files read back. A
// copy of a live mount taken without the freeze still says
// needs_recovery and goes out raw. Needs root for mount and fsfreeze:
// `go test -c`, then run the binary with sudo.
func TestFrozenRunningVolumeRoundTrip(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root for mount and fsfreeze")
	}
	needTools(t, "mkfs.ext4", "dumpe2fs", "e2fsck", "debugfs", "zstd", "fsfreeze", "mount", "cp")
	const size = 1 << 30
	img := mkfsImage(t, size, map[string][]byte{"base/old": bytes.Repeat([]byte("old "), 50000)})
	random := make([]byte, 20<<20)
	_, _ = rand.Read(random)
	files := map[string][]byte{"work/random": random}
	for i := range 300 {
		files[fmt.Sprintf("work/small/%03d", i)] = []byte(strings.Repeat(fmt.Sprint(i), 100+i))
	}
	snap := mountedCopy(t, img, true, func(mnt string) func() {
		for name, b := range files {
			p := filepath.Join(mnt, name)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, b, 0o644); err != nil { // no fsync: allocated by the freeze's sync
				t.Fatal(err)
			}
		}
		if err := os.Remove(filepath.Join(mnt, "base/old")); err != nil {
			t.Fatal(err)
		}
		held, err := os.Create(filepath.Join(mnt, "orphan"))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = held.Write(random[:1<<20])
		_ = os.Remove(held.Name()) // open and unlinked: an orphan inside the snapshot
		return func() { _ = held.Close() }
	})
	out, err := exec.Command("dumpe2fs", snap).Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "needs_recovery") {
		t.Fatal("the frozen copy still says needs_recovery")
	}
	l, err := parseDumpe2fs(out)
	if err != nil {
		t.Fatalf("the frozen copy is refused: %v", err)
	}
	dst := emptyDevice(t, size)
	p := &Pipeline{R: shell.Exec{}}
	if _, mode := roundTrip(t, p, snap, dst); mode.Format != "extents" {
		t.Fatalf("mode %+v, want extents", mode)
	}
	a, _ := os.ReadFile(snap)
	b, _ := os.ReadFile(dst)
	if len(a) != len(b) {
		t.Fatalf("sizes %d and %d", len(a), len(b))
	}
	next := uint64(0)
	checkZero := func(from, to uint64) {
		for off := from; off < to; off++ {
			if b[off] != 0 {
				t.Fatalf("byte %d, free in the bitmaps, is not zero after restore", off)
			}
		}
	}
	for _, r := range l.used {
		if !bytes.Equal(a[r[0]:r[1]], b[r[0]:r[1]]) {
			t.Fatalf("used range %d-%d differs after restore", r[0], r[1])
		}
		checkZero(next, r[0])
		next = r[1]
	}
	checkZero(next, uint64(len(b)))
	t.Logf("%d used bytes of %d round-tripped identical", l.usedBytes(), len(a))
	// e2fsck reads the restored volume exactly as it reads the snapshot
	// (the open orphan is reported the same way on both), and the restore's
	// own e2fsck -fp (lvm.Fsck) releases the orphan, as the guest's mount
	// would, and leaves nothing else to fix.
	fsck := func(dev string) string {
		out, _ := exec.Command("e2fsck", "-fn", dev).CombinedOutput()
		return strings.ReplaceAll(string(out), dev, "DEV")
	}
	if a, b := fsck(snap), fsck(dst); a != b {
		t.Fatalf("e2fsck differs between the snapshot and its restore:\n%s\n---\n%s", a, b)
	}
	if out, err := exec.Command("e2fsck", "-fp", dst).CombinedOutput(); err != nil {
		if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() > 1 {
			t.Fatalf("e2fsck -fp on the restored volume: %v\n%s", err, out)
		}
	}
	if out, err := exec.Command("e2fsck", "-fn", dst).CombinedOutput(); err != nil {
		t.Fatalf("e2fsck -fn after the restore's fsck: %v\n%s", err, out)
	}
	for name, want := range files {
		if got := debugfsCat(t, dst, "/"+name); !bytes.Equal(got, want) {
			t.Fatalf("%s differs after restore (%d vs %d bytes)", name, len(got), len(want))
		}
	}

	// The control: a live mount copied without the freeze.
	img2 := mkfsImage(t, 256<<20, map[string][]byte{"f": []byte("x")})
	unfrozen := mountedCopy(t, img2, false, func(mnt string) func() {
		if err := os.WriteFile(filepath.Join(mnt, "g"), random[:1<<20], 0o644); err != nil {
			t.Fatal(err)
		}
		return nil
	})
	if _, mode := roundTrip(t, p, unfrozen, emptyDevice(t, 256<<20)); mode.Format != "raw" || !strings.Contains(mode.Why, "recovery") {
		t.Fatalf("unfrozen copy: mode %+v, want raw because of the journal", mode)
	}
}
