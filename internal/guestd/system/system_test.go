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

	res, err := h.Switch(context.Background(), same, false)
	if err != nil {
		t.Fatalf("switch: %v", err)
	}
	if res.GetNeedsReboot() || res.GetRebooted() {
		t.Fatalf("needs_reboot=%v rebooted=%v, want both false", res.GetNeedsReboot(), res.GetRebooted())
	}
	if !strings.Contains(string(res.GetOutput()), "activating the configuration") {
		t.Fatalf("output was not returned: %q", res.GetOutput())
	}
	if _, ok := run.Ran("switch-to-configuration switch"); !ok {
		t.Fatalf("switch-to-configuration switch was not run; calls: %v", run.Calls())
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

	res, err := h.Switch(context.Background(), different, false)
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

	res, err := h.Switch(context.Background(), different, true)
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

	res, err := h.Switch(context.Background(), same, false)
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
		if _, err := h.Switch(context.Background(), closure, false); err == nil {
			t.Fatalf("%q was accepted", closure)
		} else if sysdep.CodeOf(err) != sysdep.CodeInvalidArgument {
			t.Fatalf("%q: code = %s, want invalid_argument", closure, sysdep.CodeOf(err))
		}
	}
}

func TestSwitchMissingClosureIsNotFound(t *testing.T) {
	p, _, _ := guestRoot(t)
	h := New(p, sysdep.NewFakeRunner(), 0, quietLog())

	_, err := h.Switch(context.Background(), "/nix/store/zzzz-collected-by-gc", false)
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
