package exec

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
	"github.com/heracraft/repose/internal/guestd/sysdep"
)

func quietLog() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func newHandler(t *testing.T) (*Handler, *sysdep.FakeRunner) {
	t.Helper()
	run := sysdep.NewFakeRunner()
	return New(sysdep.Paths{Root: t.TempDir()}, run, quietLog()), run
}

func TestExecRunsAsRootByDefault(t *testing.T) {
	h, run := newHandler(t)
	run.Results["uptime"] = sysdep.RunResult{Stdout: []byte(" 10:12:01 up 3 days\n")}

	res, err := h.Exec(context.Background(), &guestdv1.Exec{Argv: []string{"uptime"}})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if res.GetExitCode() != 0 || !strings.Contains(string(res.GetStdout()), "up 3 days") {
		t.Fatalf("result = %+v", res)
	}
	calls := run.Calls()
	if len(calls) != 1 || calls[0].User != "root" {
		t.Fatalf("user = %q, want root", calls[0].User)
	}
}

func TestExecRunsAsDev(t *testing.T) {
	h, run := newHandler(t)
	if _, err := h.Exec(context.Background(), &guestdv1.Exec{Argv: []string{"id"}, AsUser: "dev"}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if run.Calls()[0].User != "dev" {
		t.Fatalf("user = %q, want dev", run.Calls()[0].User)
	}
}

func TestExecReturnsNonZeroAsAResultNotAnError(t *testing.T) {
	h, run := newHandler(t)
	run.Results["false"] = sysdep.RunResult{ExitCode: 3, Stderr: []byte("nope")}

	res, err := h.Exec(context.Background(), &guestdv1.Exec{Argv: []string{"false"}})
	if err != nil {
		t.Fatalf("a non-zero exit must not be an error: %v", err)
	}
	if res.GetExitCode() != 3 {
		t.Fatalf("exit_code = %d, want 3", res.GetExitCode())
	}
	if string(res.GetStderr()) != "nope" {
		t.Fatalf("stderr = %q", res.GetStderr())
	}
}

func TestExecValidation(t *testing.T) {
	h, _ := newHandler(t)
	cases := map[string]*guestdv1.Exec{
		"empty argv":    {},
		"empty command": {Argv: []string{""}},
		"bad user":      {Argv: []string{"id"}, AsUser: "root2"},
		"too many args": {Argv: make([]string, MaxArgv+1)},
	}
	for name, req := range cases {
		if name == "too many args" {
			for i := range req.Argv {
				req.Argv[i] = "x"
			}
		}
		if _, err := h.Exec(context.Background(), req); err == nil {
			t.Errorf("%s was accepted", name)
		} else if sysdep.CodeOf(err) != sysdep.CodeInvalidArgument {
			t.Errorf("%s: code = %s, want invalid_argument", name, sysdep.CodeOf(err))
		}
	}
}

func TestExecTimeoutIsBounded(t *testing.T) {
	h, run := newHandler(t)
	// An operator typo asking for a week must not pin a process for a week.
	if _, err := h.Exec(context.Background(), &guestdv1.Exec{
		Argv: []string{"sleep"}, TimeoutS: 7 * 24 * 3600,
	}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if len(run.Calls()) != 1 {
		t.Fatalf("calls = %d", len(run.Calls()))
	}
}

func TestStreamsAreCapped(t *testing.T) {
	if StreamCap != 64<<10 {
		t.Fatalf("StreamCap = %d, want 64 KB per docs/interfaces/grpc-hostd.md", StreamCap)
	}
}
