package ssh

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/heracraft/repose/internal/guestd/sysdep"
)

func quietLog() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func newHandler(t *testing.T) (*Handler, sysdep.Paths, *sysdep.FakeRunner) {
	t.Helper()
	p := sysdep.Paths{Root: t.TempDir()}
	run := sysdep.NewFakeRunner()
	return New(p, run, quietLog()), p, run
}

func TestSetWritesPrincipalsAndReloadsSSHD(t *testing.T) {
	h, p, run := newHandler(t)
	want := []string{"01931f0e-0000-7000-8000-000000000001", "todo-app.heracraft"}

	if err := h.Set(context.Background(), want); err != nil {
		t.Fatalf("set: %v", err)
	}
	b, err := os.ReadFile(p.Principals())
	if err != nil {
		t.Fatalf("principals: %v", err)
	}
	if string(b) != strings.Join(want, "\n")+"\n" {
		t.Fatalf("principals =\n%q\nwant one per line", b)
	}
	fi, err := os.Stat(p.Principals())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %o, want 644: sshd reads this as root but it is not secret", fi.Mode().Perm())
	}
	if _, ok := run.Ran("reload-or-restart sshd.service"); !ok {
		t.Fatalf("sshd was not reloaded; calls: %v", run.Calls())
	}
}

func TestSetIsIdempotent(t *testing.T) {
	h, p, _ := newHandler(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := h.Set(ctx, []string{"p1"}); err != nil {
			t.Fatalf("set %d: %v", i, err)
		}
	}
	b, _ := os.ReadFile(p.Principals())
	if string(b) != "p1\n" {
		t.Fatalf("principals = %q after three identical calls", b)
	}
}

func TestSetEmptyListWritesAnEmptyFile(t *testing.T) {
	h, p, _ := newHandler(t)
	if err := h.Set(context.Background(), nil); err != nil {
		t.Fatalf("set: %v", err)
	}
	b, err := os.ReadFile(p.Principals())
	if err != nil {
		t.Fatalf("principals: %v", err)
	}
	// An empty file locks the guest, which is the correct reading of "no
	// principals"; a missing file would make sshd fall back to its default.
	if len(b) != 0 {
		t.Fatalf("principals = %q, want empty", b)
	}
}

func TestSetRejectsBadPrincipals(t *testing.T) {
	h, _, _ := newHandler(t)
	cases := map[string][]string{
		"empty":    {""},
		"blank":    {"   "},
		"newline":  {"good\nsneaky"},
		"too long": {strings.Repeat("x", MaxPrincipalBytes+1)},
		"too many": make([]string, MaxPrincipals+1),
	}
	for name, list := range cases {
		if name == "too many" {
			for i := range list {
				list[i] = "p"
			}
		}
		if err := h.Set(context.Background(), list); err == nil {
			t.Errorf("%s was accepted", name)
		} else if sysdep.CodeOf(err) != sysdep.CodeInvalidArgument {
			t.Errorf("%s: code = %s, want invalid_argument", name, sysdep.CodeOf(err))
		}
	}
}

func TestSetReportsAFailedReload(t *testing.T) {
	h, _, run := newHandler(t)
	run.Results["systemctl"] = sysdep.RunResult{ExitCode: 1, Stderr: []byte("Job failed")}
	if err := h.Set(context.Background(), []string{"p1"}); err == nil {
		t.Fatal("a failed sshd reload was reported as success")
	}
}
