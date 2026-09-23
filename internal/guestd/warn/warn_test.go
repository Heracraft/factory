package warn

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/guestd/sysdep"
)

func quietLog() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

type recorder struct {
	mu   sync.Mutex
	kind []string
	deta []string
}

func (r *recorder) warn(kind, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.kind = append(r.kind, kind)
	r.deta = append(r.deta, detail)
}

func (r *recorder) kinds() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.kind...)
}

func (r *recorder) details() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.deta...)
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestRateLimitIsPerKind(t *testing.T) {
	rec := &recorder{}
	clk := &clock{t: time.Now()}
	c := New(sysdep.Paths{Root: t.TempDir()}, rec.warn, quietLog(), clk.now)

	for i := 0; i < 5; i++ {
		c.send(KindDiskHigh, "93 percent")
		clk.advance(time.Minute)
	}
	if got := rec.kinds(); len(got) != 1 {
		t.Fatalf("warnings = %v, want one inside the repeat window", got)
	}

	clk.advance(Repeat)
	c.send(KindDiskHigh, "95 percent")
	if got := rec.kinds(); len(got) != 2 {
		t.Fatalf("warnings = %v, want a second one after the window", got)
	}

	// A different kind is not suppressed by the first one's window.
	c.send(KindOOM, "the kernel killed node")
	if got := rec.kinds(); len(got) != 3 {
		t.Fatalf("warnings = %v, want the other kind through", got)
	}
}

func TestInotifyExhaustion(t *testing.T) {
	root := t.TempDir()
	p := sysdep.Paths{Root: root}
	if err := os.MkdirAll(filepath.Join(root, "proc", "sys", "fs", "inotify"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A small limit and a process holding most of it.
	if err := os.WriteFile(p.InotifyMax("max_user_instances"), []byte("10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.InotifyMax("max_user_watches"), []byte("1000000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for pid := 1; pid <= 9; pid++ {
		dir := filepath.Join(p.ProcPID(fmt.Sprint(pid)), "fdinfo")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := "pos:\t0\nflags:\t02000000\ninotify wd:1 ino:1234 sdev:800001\n"
		if err := os.WriteFile(filepath.Join(dir, "5"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rec := &recorder{}
	c := New(p, rec.warn, quietLog(), time.Now)
	c.checkInotify()

	if got := rec.kinds(); len(got) != 1 || got[0] != KindInotifyExhausted {
		t.Fatalf("warnings = %v, want one %s", got, KindInotifyExhausted)
	}
	if d := rec.details(); len(d) != 1 || d[0] != "inotify instances 9 of 10" {
		t.Fatalf("detail = %v", d)
	}
}

func TestInotifyBelowTheThresholdIsQuiet(t *testing.T) {
	root := t.TempDir()
	p := sysdep.Paths{Root: root}
	if err := os.MkdirAll(filepath.Join(root, "proc", "sys", "fs", "inotify"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.InotifyMax("max_user_instances"), []byte("1024\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.InotifyMax("max_user_watches"), []byte("1048576\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	New(p, rec.warn, quietLog(), time.Now).checkInotify()
	if got := rec.kinds(); len(got) != 0 {
		t.Fatalf("warnings = %v, want none", got)
	}
}

func TestStorePathMissing(t *testing.T) {
	root := t.TempDir()
	p := sysdep.Paths{Root: root}

	system := filepath.Join(root, "nix", "store", "aaaa-system")
	binDir := filepath.Join(system, "sw", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	present := filepath.Join(root, "nix", "store", "bbbb-coreutils", "bin", "ls")
	if err := os.MkdirAll(filepath.Dir(present), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(present, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(root, "nix", "store", "cccc-collected", "bin", "node")
	if err := os.Symlink(present, filepath.Join(binDir, "ls")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gone, filepath.Join(binDir, "node")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "run"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(system, p.CurrentSystem()); err != nil {
		t.Fatal(err)
	}

	rec := &recorder{}
	c := New(p, rec.warn, quietLog(), time.Now)
	c.checkStore()

	if got := rec.kinds(); len(got) != 1 || got[0] != KindStorePathMissing {
		t.Fatalf("warnings = %v, want one %s", got, KindStorePathMissing)
	}
	if d := rec.details(); d[0] != gone {
		t.Fatalf("detail = %q, want the missing store path so the host knows what to restore", d[0])
	}
}

func TestStoreCheckIsQuietWhenEverythingIsPresent(t *testing.T) {
	root := t.TempDir()
	p := sysdep.Paths{Root: root}
	system := filepath.Join(root, "nix", "store", "aaaa-system")
	binDir := filepath.Join(system, "sw", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	present := filepath.Join(root, "nix", "store", "bbbb-coreutils", "bin", "ls")
	if err := os.MkdirAll(filepath.Dir(present), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(present, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(present, filepath.Join(binDir, "ls")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "run"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(system, p.CurrentSystem()); err != nil {
		t.Fatal(err)
	}

	rec := &recorder{}
	New(p, rec.warn, quietLog(), time.Now).checkStore()
	if got := rec.kinds(); len(got) != 0 {
		t.Fatalf("warnings = %v, want none", got)
	}
}

func TestDiskHighUsesTheRootMount(t *testing.T) {
	// The temp directory is on a real filesystem, so this exercises statfs and
	// the percentage arithmetic; whether it warns depends on the test machine,
	// so only the absence of a crash and the kind are asserted.
	rec := &recorder{}
	c := New(sysdep.Paths{Root: t.TempDir()}, rec.warn, quietLog(), time.Now)
	c.checkDisk()
	for _, k := range rec.kinds() {
		if k != KindDiskHigh {
			t.Fatalf("unexpected warning kind %q", k)
		}
	}
}

func TestOOMProcessExtraction(t *testing.T) {
	cases := map[string]string{
		"6,1234,567,-;Out of memory: Killed process 4242 (node) total-vm:...": "the kernel killed node",
		"6,1,1,-;oom-kill:constraint=CONSTRAINT_NONE,...,task=cargo,pid=99":   "the kernel killed cargo",
		"6,1,1,-;something else entirely":                                     "the kernel reported an out-of-memory condition",
		"6,1,1,-;Memory cgroup out of memory: Killed process 1 (pnpm)":        "the kernel killed pnpm",
	}
	for line, want := range cases {
		if got := oomProcess(line); got != want {
			t.Errorf("oomProcess(%q) = %q, want %q", line, got, want)
		}
	}
}

// The kernel's own order for one kill: the process that asked for memory
// "invoked oom-killer", then the kill lines. The one warning the rate
// limit allows names the killed process, unwrapped from nix's name
// (DECISIONS I-213).
func TestOOMWarningNamesTheKilledProcess(t *testing.T) {
	rec := &recorder{}
	c := New(sysdep.Paths{Root: t.TempDir()}, rec.warn, quietLog(), time.Now)
	for _, line := range []string{
		"4,900,1,-;node invoked oom-killer: gfp_mask=0x140cca(GFP_HIGHUSER_MOVABLE|__GFP_COMP), order=0, oom_score_adj=0",
		"6,901,1,-;oom-kill:constraint=CONSTRAINT_NONE,nodemask=(null),cpuset=/,mems_allowed=0,global_oom,task_memcg=/user.slice,task=.claude-wrapped,pid=4242,uid=1000",
		"3,902,1,-;Out of memory: Killed process 4242 (.claude-wrapped) total-vm:9999kB, anon-rss:1kB",
	} {
		c.kmsgLine(line)
	}
	if got := rec.details(); len(got) != 1 || got[0] != "the kernel killed claude" {
		t.Fatalf("warnings = %q, want one naming claude", got)
	}
}

func TestKmsgIsDisabledUnderATestRoot(t *testing.T) {
	c := New(sysdep.Paths{Root: t.TempDir()}, func(string, string) {}, quietLog(), time.Now)
	if c.KmsgPath != "" {
		t.Fatalf("KmsgPath = %q, want empty under a test root", c.KmsgPath)
	}
}
