package system

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/heracraft/repose/internal/guestd/sysdep"
)

// I-225: a registration this volume's database already took is not
// loaded again (the stamp the boot waits for is still written); a new
// one, a failed one, or a missing database is loaded.
func TestRegisterPathsSkipsARegistrationAlreadyLoaded(t *testing.T) {
	p, _, _ := guestRoot(t)
	if err := os.MkdirAll(p.RunDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	run := sysdep.NewFakeRunner()
	h := New(p, run, 0, quietLog())
	ctx := context.Background()
	loads := func() int {
		n := 0
		for _, c := range run.Calls() {
			if len(c.Argv) > 1 && c.Argv[0] == "nix-store" && c.Argv[1] == "--load-db" {
				n++
			}
		}
		return n
	}
	reg := []byte("/nix/store/aaaa-x\n\n0\n")

	if err := h.RegisterPaths(ctx, reg); err != nil {
		t.Fatal(err)
	}
	if loads() != 1 {
		t.Fatalf("first registration: %d loads", loads())
	}
	// No database yet (the fake load wrote none): loaded again.
	_ = os.Remove(p.PathsRegistered())
	if err := h.RegisterPaths(ctx, reg); err != nil {
		t.Fatal(err)
	}
	if loads() != 2 {
		t.Fatalf("no database: %d loads", loads())
	}
	// The next boot of the same volume, same closure: skipped, stamped.
	if err := os.MkdirAll(filepath.Dir(p.NixDB()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.NixDB(), []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(p.PathsRegistered())
	if err := h.RegisterPaths(ctx, reg); err != nil {
		t.Fatal(err)
	}
	if loads() != 2 {
		t.Fatalf("same registration: %d loads, want no new one", loads())
	}
	if _, err := os.Stat(p.PathsRegistered()); err != nil {
		t.Fatalf("stamp not written on a skip: %v", err)
	}
	// Another closure: loaded.
	if err := h.RegisterPaths(ctx, []byte("/nix/store/bbbb-y\n\n0\n")); err != nil {
		t.Fatal(err)
	}
	if loads() != 3 {
		t.Fatalf("new registration: %d loads", loads())
	}
	// A load that fails is not recorded: the same one is tried again.
	run.Match["nix-store --load-db"] = sysdep.RunResult{ExitCode: 1}
	failing := []byte("/nix/store/cccc-z\n\n0\n")
	if err := h.RegisterPaths(ctx, failing); err == nil {
		t.Fatal("a failed load succeeded")
	}
	delete(run.Match, "nix-store --load-db")
	if err := h.RegisterPaths(ctx, failing); err != nil {
		t.Fatal(err)
	}
	if loads() != 5 {
		t.Fatalf("after a failed load: %d loads, want 5", loads())
	}
}
