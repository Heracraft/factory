package systemd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/hostd/shell"
)

// deadlineRunner answers is-active with "active" until ctx ends, and then
// the way a systemctl killed by the deadline does: a non-zero exit and no
// output.
type deadlineRunner struct{ shell.Runner }

func (deadlineRunner) Run(ctx context.Context, argv ...string) (shell.Result, error) {
	if ctx.Err() != nil {
		return shell.Result{ExitCode: -1}, &shell.ExitError{Argv: argv, Result: shell.Result{ExitCode: -1}}
	}
	return shell.Result{Stdout: []byte("active\n")}, nil
}

// TestWaitInactiveTimeoutIsNotInactive: a guest that never powered off
// read as stopped once the wait's deadline killed systemctl, so the stop
// skipped its fallback and logged nothing (host-01, 04:11:19, I-186).
func TestWaitInactiveTimeoutIsNotInactive(t *testing.T) {
	s := &Real{R: deadlineRunner{}, Poll: 10 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := s.WaitInactive(ctx, "guest@g1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitInactive past its deadline: %v, want DeadlineExceeded", err)
	}
}
