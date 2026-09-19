package fs

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/heracraft/repose/internal/guestd/sysdep"
)

func quietLog() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func rootWithMounts(t *testing.T, mounts string) sysdep.Paths {
	t.Helper()
	root := t.TempDir()
	p := sysdep.Paths{Root: root}
	if err := os.MkdirAll(p.Proc(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Mounts(), []byte(mounts), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const realisticMounts = `devtmpfs /dev devtmpfs rw,nosuid 0 0
/dev/vda / ext4 rw,relatime 0 0
tmpfs /run/repose tmpfs rw,nosuid,nodev 0 0
ro-store /nix/.ro-store virtiofs ro,relatime 0 0
`

func TestGrowRunsResize2fsOnTheRootDevice(t *testing.T) {
	p := rootWithMounts(t, realisticMounts)
	run := sysdep.NewFakeRunner()
	h := New(p, run, 0, quietLog())

	res, err := h.Grow(context.Background())
	if err != nil {
		t.Fatalf("grow: %v", err)
	}
	call, ok := run.Ran("resize2fs")
	if !ok {
		t.Fatalf("resize2fs was not run; calls: %v", run.Calls())
	}
	if len(call.Argv) != 2 || call.Argv[1] != "/dev/vda" {
		t.Fatalf("argv = %v, want [resize2fs /dev/vda]", call.Argv)
	}
	// statfs of the temp dir is a real filesystem, so the size is non-zero.
	if res.GetNewBytes() == 0 {
		t.Fatal("new_bytes was zero")
	}
}

func TestGrowFailsWhenResize2fsFails(t *testing.T) {
	p := rootWithMounts(t, realisticMounts)
	run := sysdep.NewFakeRunner()
	run.Results["resize2fs"] = sysdep.RunResult{ExitCode: 1, Stderr: []byte("bad magic number")}
	h := New(p, run, 0, quietLog())

	if _, err := h.Grow(context.Background()); err == nil {
		t.Fatal("a failing resize2fs was reported as success")
	}
}

func TestGrowRefusesANonBlockRoot(t *testing.T) {
	p := rootWithMounts(t, "overlay / overlay rw 0 0\n")
	h := New(p, sysdep.NewFakeRunner(), 0, quietLog())

	_, err := h.Grow(context.Background())
	if err == nil {
		t.Fatal("an overlay root was accepted")
	}
	if sysdep.CodeOf(err) != sysdep.CodeInvalidArgument {
		t.Fatalf("code = %s, want invalid_argument", sysdep.CodeOf(err))
	}
}

func TestGrowWithNoRootMount(t *testing.T) {
	p := rootWithMounts(t, "devtmpfs /dev devtmpfs rw 0 0\n")
	h := New(p, sysdep.NewFakeRunner(), 0, quietLog())

	_, err := h.Grow(context.Background())
	if sysdep.CodeOf(err) != sysdep.CodeNotFound {
		t.Fatalf("code = %s, want not_found", sysdep.CodeOf(err))
	}
}

func TestGrowWithUnreadableMounts(t *testing.T) {
	p := sysdep.Paths{Root: filepath.Join(t.TempDir(), "absent")}
	h := New(p, sysdep.NewFakeRunner(), 0, quietLog())
	if _, err := h.Grow(context.Background()); err == nil {
		t.Fatal("a missing /proc/mounts was not reported")
	}
}
