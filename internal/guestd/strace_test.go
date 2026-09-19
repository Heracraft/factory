//go:build linux

package guestd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
	"github.com/heracraft/repose/internal/vsockrpc"
)

// TestStraceNeverOpensCmdlineOrEnviron is the enforcement of DECISIONS R5-3
// and of the sentence in the privacy policy. guestd is run under strace while
// a Sample and a hook are served, and the syscall log must contain no open of
// any /proc/<pid>/cmdline and no open of any /proc/<pid>/environ other than
// the one documented read of a hook caller's TMUX_PANE.
//
// The reason this is a test and not a code review: the boundary is a promise
// made in the privacy policy, and a library added later that reads a process
// command line would keep every unit test green.
func TestStraceNeverOpensCmdlineOrEnviron(t *testing.T) {
	_, devSock, hookSock, tracePath, stop := tracedGuestd(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := dialWithRetry(t, devSock)
	defer client.Close() //nolint:errcheck // test cleanup

	// A Sample walks every process in /proc. This is the request that would
	// read a command line if anything did.
	resp, err := client.Do(ctx, &guestdv1.Request{Req: &guestdv1.Request_Sample{Sample: &guestdv1.Sample{}}})
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	if !resp.GetOk() {
		t.Fatalf("sample: %+v", resp.GetError())
	}
	if len(resp.GetSample().GetProcs()) == 0 {
		t.Fatal("the sample walked no processes, so the trace proves nothing")
	}

	// A hook that names its own window: the TMUX_PANE lookup must not happen.
	postHook(t, hookSock, `{"agent":"claude","kind":"completed","summary":"x","window":"claude"}`)
	time.Sleep(300 * time.Millisecond)
	stop()

	cmdlines, environs := procOpens(t, tracePath)
	if len(cmdlines) > 0 {
		t.Fatalf("guestd opened a process command line, which the privacy policy says it never does:\n%s",
			strings.Join(cmdlines, "\n"))
	}
	if len(environs) > 0 {
		t.Fatalf("guestd opened a process environment while serving a Sample and a hook that named its window:\n%s",
			strings.Join(environs, "\n"))
	}
}

// TestStraceReadsEnvironOnlyForTheHookPaneLookup is the other half: when a
// hook does not name its window, exactly one environment is read, and it is
// the caller's own.
func TestStraceReadsEnvironOnlyForTheHookPaneLookup(t *testing.T) {
	_, _, hookSock, tracePath, stop := tracedGuestd(t)

	waitForSocket(t, hookSock)
	postHook(t, hookSock, `{"agent":"claude","kind":"completed","summary":"x"}`)
	time.Sleep(300 * time.Millisecond)
	stop()

	cmdlines, environs := procOpens(t, tracePath)
	if len(cmdlines) > 0 {
		t.Fatalf("guestd opened a process command line:\n%s", strings.Join(cmdlines, "\n"))
	}
	if len(environs) != 1 {
		t.Fatalf("environ opens = %d, want exactly the one documented TMUX_PANE read:\n%s",
			len(environs), strings.Join(environs, "\n"))
	}
}

// tracedGuestd starts guestd under strace over a guest root whose /proc is the
// real one, and returns the trace path and a stop function. The whole process
// group is killed on stop: strace -f leaves its child running otherwise, and a
// leaked guestd would outlive the test.
func tracedGuestd(t *testing.T) (root, devSock, hookSock, tracePath string, stop func()) {
	t.Helper()
	strace, err := exec.LookPath("strace")
	if err != nil {
		t.Skip("strace is not installed; this test must run on the Linux CI runner")
	}

	root = t.TempDir()
	// The sampler must walk a real process table for the trace to prove
	// anything, so the fake root's /proc is the real /proc.
	if err := os.Symlink("/proc", filepath.Join(root, "proc")); err != nil {
		t.Fatalf("link /proc into the test root: %v", err)
	}
	devSock = filepath.Join(root, "run", "repose", "guestd.sock")
	hookSock = filepath.Join(root, "run", "repose", "hooks.sock")
	tracePath = filepath.Join(t.TempDir(), "trace.log")

	cmd := exec.Command(strace,
		"-f", "-e", "trace=openat,open", "-o", tracePath,
		buildGuestd(t),
		"--root", root,
		"--dev-socket", devSock,
		"--hook-socket", hookSock,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start guestd under strace: %v", err)
	}

	var once bool
	stop = func() {
		if once {
			return
		}
		once = true
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	}
	t.Cleanup(stop)
	return root, devSock, hookSock, tracePath, stop
}

// procOpens splits a trace into the command-line and environment opens of
// process directories, which are the two things guestd must not do.
func procOpens(t *testing.T, tracePath string) (cmdlines, environs []string) {
	t.Helper()
	trace, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read the trace: %v", err)
	}
	if !strings.Contains(string(trace), "/proc/") {
		t.Fatal("the trace contains no /proc access at all, so the filter did not capture what it should and the test proves nothing")
	}
	for _, line := range strings.Split(string(trace), "\n") {
		if !strings.Contains(line, "/proc/") {
			continue
		}
		if strings.Contains(line, "/cmdline") {
			cmdlines = append(cmdlines, line)
		}
		if strings.Contains(line, "/environ") {
			environs = append(environs, line)
		}
	}
	return cmdlines, environs
}

func buildGuestd(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "guestd")
	cmd := exec.Command("go", "build", "-o", out, "github.com/heracraft/repose/cmd/guestd")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build guestd: %v\n%s", err, b)
	}
	return out
}

func dialWithRetry(t *testing.T, path string) *vsockrpc.Client {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		conn, err := vsockrpc.DialUnix(path)
		if err == nil {
			return vsockrpc.NewClient(conn, nil)
		}
		if time.Now().After(deadline) {
			t.Fatalf("dial %s: %v", path, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", filepath.Base(path))
}
