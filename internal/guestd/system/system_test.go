package system

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heracraft/repose/internal/guestd/sysdep"
)

func quietLog() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

// guestRoot builds a fake guest: a store with two system closures and a
// /run/current-system pointing at the first.
func guestRoot(t *testing.T) (sysdep.Paths, string, string) {
	t.Helper()
	root := t.TempDir()
	p := sysdep.Paths{Root: root}

	mkSystem := func(name, kernel string) string {
		store := "/nix/store/" + name
		dir := filepath.Join(root, store)
		if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		kernelPath := filepath.Join(root, "/nix/store/"+kernel+"/bzImage")
		if err := os.MkdirAll(filepath.Dir(kernelPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(kernelPath, []byte("kernel"), 0o644); err != nil {
			t.Fatal(err)
		}
		initrdPath := filepath.Join(root, "/nix/store/"+kernel+"/initrd")
		if err := os.WriteFile(initrdPath, []byte("initrd"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(kernelPath, filepath.Join(dir, "kernel")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(initrdPath, filepath.Join(dir, "initrd")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "bin", "switch-to-configuration"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return store
	}

	same := mkSystem("aaaa-nixos-system-same", "kkkk-kernel")
	different := mkSystem("bbbb-nixos-system-newkernel", "jjjj-kernel")

	if err := os.MkdirAll(filepath.Join(root, "run"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, same), p.CurrentSystem()); err != nil {
		t.Fatal(err)
	}
	return p, same, different
}

func TestSwitchAppliesWithoutReboot(t *testing.T) {
	p, same, _ := guestRoot(t)
	run := sysdep.NewFakeRunner()
	run.Results["switch-to-configuration"] = sysdep.RunResult{Stdout: []byte("activating the configuration...\n")}
	h := New(p, run, 0, quietLog())

	res, err := h.Switch(context.Background(), same, false, nil)
	if err != nil {
		t.Fatalf("switch: %v", err)
	}
	if res.GetNeedsReboot() || res.GetRebooted() {
		t.Fatalf("needs_reboot=%v rebooted=%v, want both false", res.GetNeedsReboot(), res.GetRebooted())
	}
	if !strings.Contains(string(res.GetOutput()), "activating the configuration") {
		t.Fatalf("output was not returned: %q", res.GetOutput())
	}
	call, ok := run.Ran("switch-to-configuration switch")
	if !ok {
		t.Fatalf("switch-to-configuration switch was not run; calls: %v", run.Calls())
	}
	// The activation is a transient unit of its own, not guestd's child
	// (I-143), waited for with its output piped back.
	if len(call.Argv) < 6 || call.Argv[0] != "systemd-run" || call.Argv[1] != "--wait" || call.Argv[2] != "--pipe" {
		t.Fatalf("switch not run through systemd-run --wait --pipe: %v", call.Argv)
	}
	// The same binary: no restart of guestd scheduled.
	if _, ok := run.Ran("systemctl restart guestd"); ok {
		t.Fatalf("a restart was scheduled although guestd did not change: %v", run.Calls())
	}
	// The profile is set before the activation so a later reboot lands on the
	// new system.
	if _, ok := run.Ran("nix-env --profile"); !ok {
		t.Fatal("the system profile was not set")
	}
}

func TestSwitchRefusesWhenKernelChanged(t *testing.T) {
	p, _, different := guestRoot(t)
	run := sysdep.NewFakeRunner()
	h := New(p, run, 0, quietLog())

	res, err := h.Switch(context.Background(), different, false, nil)
	if err != nil {
		t.Fatalf("switch: %v", err)
	}
	if !res.GetNeedsReboot() {
		t.Fatal("needs_reboot was not set for a closure with a different kernel")
	}
	if res.GetRebooted() {
		t.Fatal("rebooted was set")
	}
	if len(run.Calls()) != 0 {
		t.Fatalf("nothing should have run, but %v did", run.Calls())
	}
}

func TestSwitchForceRebootActivatesForBoot(t *testing.T) {
	p, _, different := guestRoot(t)
	run := sysdep.NewFakeRunner()
	h := New(p, run, 0, quietLog())
	h.rebootArgv = []string{"true"} // do not reboot the test machine

	res, err := h.Switch(context.Background(), different, true, nil)
	if err != nil {
		t.Fatalf("switch: %v", err)
	}
	if !res.GetRebooted() {
		t.Fatal("rebooted was not set")
	}
	if _, ok := run.Ran("switch-to-configuration boot"); !ok {
		t.Fatalf("boot activation was not run; calls: %v", run.Calls())
	}
}

func TestSwitchFailureReturnsOutputAndLeavesOldSystem(t *testing.T) {
	p, same, _ := guestRoot(t)
	run := sysdep.NewFakeRunner()
	run.Results["switch-to-configuration"] = sysdep.RunResult{
		ExitCode: 1,
		Stderr:   []byte("error: systemd unit nginx.service failed\n"),
	}
	h := New(p, run, 0, quietLog())

	res, err := h.Switch(context.Background(), same, false, nil)
	if err == nil {
		t.Fatal("a non-zero switch-to-configuration must be an error")
	}
	if sysdep.CodeOf(err) != sysdep.CodeInternal {
		t.Fatalf("code = %s, want internal", sysdep.CodeOf(err))
	}
	if !strings.Contains(string(res.GetOutput()), "nginx.service failed") {
		t.Fatalf("the failure output must come back: %q", res.GetOutput())
	}
}

func TestSwitchRejectsPathsOutsideTheStore(t *testing.T) {
	p, _, _ := guestRoot(t)
	h := New(p, sysdep.NewFakeRunner(), 0, quietLog())

	for _, closure := range []string{"", "/etc/passwd", "/nix/store/../etc"} {
		if _, err := h.Switch(context.Background(), closure, false, nil); err == nil {
			t.Fatalf("%q was accepted", closure)
		} else if sysdep.CodeOf(err) != sysdep.CodeInvalidArgument {
			t.Fatalf("%q: code = %s, want invalid_argument", closure, sysdep.CodeOf(err))
		}
	}
}

func TestSwitchMissingClosureIsNotFound(t *testing.T) {
	p, _, _ := guestRoot(t)
	h := New(p, sysdep.NewFakeRunner(), 0, quietLog())

	_, err := h.Switch(context.Background(), "/nix/store/zzzz-collected-by-gc", false, nil)
	if err == nil {
		t.Fatal("a closure that is not in the share was accepted")
	}
	if sysdep.CodeOf(err) != sysdep.CodeNotFound {
		t.Fatalf("code = %s, want not_found", sysdep.CodeOf(err))
	}
}

func TestOutputIsCappedToTheTail(t *testing.T) {
	long := strings.Repeat("a", OutputCap+500)
	out := combine([]byte(long), []byte("the end"), OutputCap)
	if len(out) != OutputCap {
		t.Fatalf("len = %d, want %d", len(out), OutputCap)
	}
	if !strings.HasSuffix(string(out), "the end") {
		t.Fatal("the tail, which says why, was not the part kept")
	}
}

// A switch to a system carrying another guestd binary schedules guestd's
// own restart from a transient unit 3 s later (I-143); the activation no
// longer restarts it (restartIfChanged = false), which on host-01 killed
// the switch and left three guests without guestd.
func TestSwitchSchedulesGuestdRestartWhenItsBinaryChanged(t *testing.T) {
	p, same, _ := guestRoot(t)
	unitDir := filepath.Join(p.Root, same, "etc", "systemd", "system") // under the fixture root, never the real store
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	unit := "[Service]\nExecStart=/nix/store/zzzz-guestd-next/bin/guestd\nRestart=always\n"
	if err := os.WriteFile(filepath.Join(unitDir, "guestd.service"), []byte(unit), 0o644); err != nil {
		t.Fatal(err)
	}
	run := sysdep.NewFakeRunner()
	run.Results["switch-to-configuration"] = sysdep.RunResult{Stdout: []byte("ok\n")}
	h := New(p, run, 0, quietLog())
	if _, err := h.Switch(context.Background(), same, false, nil); err != nil {
		t.Fatalf("switch: %v", err)
	}
	call, ok := run.Ran("systemctl restart guestd.service")
	if !ok {
		t.Fatalf("no guestd restart scheduled; calls: %v", run.Calls())
	}
	joined := strings.Join(call.Argv, " ")
	if !strings.HasPrefix(joined, "systemd-run ") || !strings.Contains(joined, "--on-active=3") || !strings.Contains(joined, "--unit repose-guestd-restart") {
		t.Fatalf("restart must be a delayed transient unit: %s", joined)
	}
	// The unit naming this very binary: nothing scheduled.
	self, _ := os.Executable()
	self, _ = filepath.EvalSymlinks(self)
	if err := os.WriteFile(filepath.Join(unitDir, "guestd.service"), []byte("[Service]\nExecStart="+self+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run.Reset()
	if _, err := h.Switch(context.Background(), same, false, nil); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if _, ok := run.Ran("systemctl restart guestd"); ok {
		t.Fatalf("restart scheduled for the same binary: %v", run.Calls())
	}
}
