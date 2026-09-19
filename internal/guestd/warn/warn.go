// Package warn is guestd's background health check: the periodic conditions
// that become Warning notifications on the vsock stream.
//
// Every kind is rate limited to one in ten minutes, because a warning that
// repeats every thirty seconds trains the operator to ignore it.
package warn

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/heracraft/repose/internal/guestd/sysdep"
	"golang.org/x/sys/unix"
)

// Warning kinds produced here. The full enumeration across guestd and hostd is
// in DECISIONS I-11 and docs/interfaces/vsock-guestd.md.
const (
	KindDiskHigh         = "disk_high"
	KindInotifyExhausted = "inotify_exhausted"
	KindStorePathMissing = "store_path_missing"
	KindOOM              = "oom"
)

// Thresholds and periods.
const (
	// DiskHighPercent is the root filesystem usage that warrants a warning.
	DiskHighPercent = 90
	// InotifyHighPercent is the share of the inotify limits that does.
	InotifyHighPercent = 90
	// Tick is the fast check: disk and the kernel log.
	Tick = 30 * time.Second
	// SlowTick is the expensive check: inotify accounting and the store.
	SlowTick = 5 * time.Minute
	// Repeat is the minimum gap between two warnings of the same kind.
	Repeat = 10 * time.Minute
	// StorePathsPerCheck bounds how many store paths one slow tick verifies,
	// so the check costs the same on a small closure and a large one.
	StorePathsPerCheck = 256
)

// Warner receives a warning. The server's notify queue implements it.
type Warner func(kind, detail string)

// Checker runs the periodic checks.
type Checker struct {
	paths sysdep.Paths
	warn  Warner
	log   *slog.Logger
	now   func() time.Time

	// KmsgPath is the kernel log read for OOM lines. Empty disables the check,
	// which is what a test root does.
	KmsgPath string

	mu       sync.Mutex
	lastSent map[string]time.Time
	// storeCursor rotates through the running system's store paths.
	storeCursor int
	storePaths  []string
}

// New builds a checker.
func New(p sysdep.Paths, warn Warner, log *slog.Logger, now func() time.Time) *Checker {
	if now == nil {
		now = time.Now
	}
	kmsg := "/dev/kmsg"
	if p.Root != "" {
		kmsg = ""
	}
	return &Checker{
		paths:    p,
		warn:     warn,
		log:      log,
		now:      now,
		KmsgPath: kmsg,
		lastSent: map[string]time.Time{},
	}
}

// Run checks until ctx is done.
func (c *Checker) Run(ctx context.Context) {
	fast := time.NewTicker(Tick)
	defer fast.Stop()
	slow := time.NewTicker(SlowTick)
	defer slow.Stop()

	if c.KmsgPath != "" {
		go c.watchKmsg(ctx)
	}
	c.Fast()
	c.Slow()
	for {
		select {
		case <-ctx.Done():
			return
		case <-fast.C:
			c.Fast()
		case <-slow.C:
			c.Slow()
		}
	}
}

// Fast is the thirty-second check. It is exported so tests drive it directly.
func (c *Checker) Fast() {
	c.checkDisk()
}

// Slow is the five-minute check.
func (c *Checker) Slow() {
	c.checkInotify()
	c.checkStore()
}

// checkDisk warns when the root filesystem is over DiskHighPercent full. A
// full root fails every write in the guest; guestd survives because its own
// state is on tmpfs, which is exactly why it can still say so.
func (c *Checker) checkDisk() {
	var st unix.Statfs_t
	if err := unix.Statfs(c.paths.RootMount(), &st); err != nil {
		c.log.Warn("could not statfs the root filesystem", "event", "warning", "kind", KindDiskHigh)
		return
	}
	if st.Blocks == 0 {
		return
	}
	used := st.Blocks - st.Bavail
	pct := int(used * 100 / st.Blocks)
	if pct < DiskHighPercent {
		return
	}
	c.send(KindDiskHigh, fmt.Sprintf("root filesystem is %d percent full", pct))
}

// checkInotify counts the inotify instances and watches in use against the
// kernel limits. An agent with a file watcher in every project directory is
// the usual way a guest runs out, and the symptom without this warning is a
// build tool that silently stops noticing edits.
func (c *Checker) checkInotify() {
	instances, watches, ok := c.countInotify()
	if !ok {
		return
	}
	maxInstances := readIntFile(c.paths.InotifyMax("max_user_instances"))
	maxWatches := readIntFile(c.paths.InotifyMax("max_user_watches"))

	if maxInstances > 0 && instances*100/maxInstances >= InotifyHighPercent {
		c.send(KindInotifyExhausted,
			fmt.Sprintf("inotify instances %d of %d", instances, maxInstances))
		return
	}
	if maxWatches > 0 && watches*100/maxWatches >= InotifyHighPercent {
		c.send(KindInotifyExhausted,
			fmt.Sprintf("inotify watches %d of %d", watches, maxWatches))
	}
}

// countInotify walks /proc/*/fdinfo counting inotify file descriptors and the
// watch descriptors inside them. It opens fdinfo, never cmdline or environ.
func (c *Checker) countInotify() (instances, watches int, ok bool) {
	entries, err := os.ReadDir(c.paths.Proc())
	if err != nil {
		return 0, 0, false
	}
	for _, e := range entries {
		if !isPID(e.Name()) {
			continue
		}
		dir := filepath.Join(c.paths.ProcPID(e.Name()), "fdinfo")
		fds, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			b, err := os.ReadFile(filepath.Join(dir, fd.Name()))
			if err != nil {
				continue
			}
			n := strings.Count(string(b), "inotify wd:")
			if n == 0 {
				continue
			}
			instances++
			watches += n
		}
	}
	return instances, watches, true
}

// checkStore verifies that store paths the running system depends on are still
// present in the share. They vanish when the host garbage-collects a closure a
// guest is still using, and the guest then fails in ways that look like
// anything but the real cause.
func (c *Checker) checkStore() {
	paths := c.loadStorePaths()
	if len(paths) == 0 {
		return
	}
	c.mu.Lock()
	start := c.storeCursor
	c.mu.Unlock()

	missing := ""
	checked := 0
	for i := 0; i < len(paths) && checked < StorePathsPerCheck; i++ {
		p := paths[(start+i)%len(paths)]
		checked++
		if _, err := os.Stat(p); err != nil && os.IsNotExist(err) {
			missing = p
			break
		}
	}

	c.mu.Lock()
	c.storeCursor = (start + checked) % len(paths)
	c.mu.Unlock()

	if missing != "" {
		// A store path is platform data, not a tenant's file path, and it is
		// the only thing that makes this warning actionable on the host.
		c.send(KindStorePathMissing, missing)
	}
}

// loadStorePaths collects the store paths the running system points at: the
// closure itself, its kernel and initrd, and the targets of everything in its
// sw/bin. That is a bounded, cheap stand-in for the full closure, which the
// guest has no nix database to enumerate.
func (c *Checker) loadStorePaths() []string {
	c.mu.Lock()
	if len(c.storePaths) > 0 {
		out := c.storePaths
		c.mu.Unlock()
		return out
	}
	c.mu.Unlock()

	cur := c.paths.CurrentSystem()
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		if !strings.HasPrefix(strings.TrimPrefix(p, c.paths.Root), "/nix/store/") {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, name := range []string{"", "kernel", "initrd"} {
		if target, err := os.Readlink(filepath.Join(cur, name)); err == nil {
			add(absolute(cur, name, target))
		}
	}
	binDir := filepath.Join(cur, "sw", "bin")
	if entries, err := os.ReadDir(binDir); err == nil {
		for _, e := range entries {
			target, err := os.Readlink(filepath.Join(binDir, e.Name()))
			if err != nil {
				continue
			}
			add(absolute(binDir, e.Name(), target))
		}
	}

	c.mu.Lock()
	c.storePaths = out
	c.mu.Unlock()
	return out
}

func absolute(dir, name, target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(dir, filepath.Dir(name), target)
}

// watchKmsg follows the kernel log from the current end and warns on OOM
// kills. Reading /dev/kmsg costs no process, unlike running dmesg.
func (c *Checker) watchKmsg(ctx context.Context) {
	f, err := os.Open(c.KmsgPath)
	if err != nil {
		c.log.Debug("kernel log is not readable; OOM warnings are off", "event", "warning", "kind", KindOOM)
		return
	}
	defer f.Close() //nolint:errcheck // read-only follow
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return
	}
	go func() {
		<-ctx.Done()
		_ = f.Close()
	}()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 8<<10), 64<<10)
	for sc.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := sc.Text()
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "out of memory") && !strings.Contains(lower, "oom-kill") {
			continue
		}
		// The kernel names the killed process; that is a process name, which
		// is on the allowed side of the sampling boundary. Nothing else from
		// the line is forwarded.
		c.send(KindOOM, oomProcess(line))
	}
}

// oomProcess extracts the killed process name from a kernel OOM line.
func oomProcess(line string) string {
	for _, key := range []string{"name=", "Killed process "} {
		if i := strings.Index(line, key); i >= 0 {
			rest := line[i+len(key):]
			rest = strings.TrimSpace(rest)
			if j := strings.IndexAny(rest, ",) \t"); j > 0 {
				rest = rest[:j]
			}
			if rest != "" {
				return "the kernel killed " + strings.Trim(rest, "()")
			}
		}
	}
	return "the kernel reported an out-of-memory condition"
}

// send rate limits and emits.
func (c *Checker) send(kind, detail string) {
	now := c.now()
	c.mu.Lock()
	last, ok := c.lastSent[kind]
	if ok && now.Sub(last) < Repeat {
		c.mu.Unlock()
		return
	}
	c.lastSent[kind] = now
	c.mu.Unlock()

	c.log.Warn(detail, "event", "warning", "kind", kind)
	c.warn(kind, detail)
}

func readIntFile(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0
	}
	return n
}

func isPID(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			return false
		}
	}
	return true
}
