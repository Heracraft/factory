// Package exec implements the Exec request of
// docs/interfaces/vsock-guestd.md. It is operator-only: hostd audits every
// call and holds the record of what was run. guestd logs that an exec
// happened and how long the argv was, never the argv itself.
package exec

import (
	"context"
	"log/slog"
	"time"

	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
	"github.com/heracraft/repose/internal/guestd/sysdep"
)

// StreamCap is the cap on each of stdout and stderr
// (docs/interfaces/grpc-hostd.md: 64 KB each).
const StreamCap = 64 << 10

// DefaultTimeout applies when the request does not set one.
const DefaultTimeout = 30 * time.Second

// MaxTimeout bounds what a request may ask for, so an operator typo cannot
// pin a goroutine and a process in the guest forever.
const MaxTimeout = 10 * time.Minute

// MaxArgv bounds the number of arguments.
const MaxArgv = 256

// Handler runs commands on behalf of an operator.
type Handler struct {
	paths sysdep.Paths
	run   sysdep.Runner
	log   *slog.Logger
}

// New builds the handler.
func New(p sysdep.Paths, run sysdep.Runner, log *slog.Logger) *Handler {
	return &Handler{paths: p, run: run, log: log}
}

// Exec runs req's argv as dev or root and returns the exit code and the
// capped output.
func (h *Handler) Exec(ctx context.Context, req *guestdv1.Exec) (*guestdv1.ExecResult, error) {
	argv := req.GetArgv()
	switch {
	case len(argv) == 0:
		return nil, sysdep.Invalid("exec: argv is empty")
	case len(argv) > MaxArgv:
		return nil, sysdep.Invalid("exec: argv has more than %d elements", MaxArgv)
	case argv[0] == "":
		return nil, sysdep.Invalid("exec: the command is empty")
	}
	user := req.GetAsUser()
	if user == "" {
		user = "root"
	}
	if user != "dev" && user != "root" {
		return nil, sysdep.Invalid("exec: as_user must be dev or root")
	}

	timeout := DefaultTimeout
	if s := req.GetTimeoutS(); s > 0 {
		timeout = time.Duration(s) * time.Second
	}
	if timeout > MaxTimeout {
		timeout = MaxTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// The argv itself is a tenant's or an operator's business and never
	// reaches a log field; its length is enough to correlate with hostd's
	// audit record.
	h.log.Info("exec requested", "event", "exec", "argv_len", len(argv), "as_user", user)

	res, err := h.run.Run(ctx, sysdep.RunSpec{
		Argv:      argv,
		User:      user,
		Env:       sysdep.DevEnv(h.paths, user),
		MaxOutput: StreamCap,
	})
	out := &guestdv1.ExecResult{
		ExitCode: int32(res.ExitCode),
		Stdout:   res.Stdout,
		Stderr:   res.Stderr,
	}
	if err != nil {
		if ctx.Err() != nil {
			return out, sysdep.Errf(sysdep.CodeInternal, "exec: timed out after %s", timeout)
		}
		return out, sysdep.Errf(sysdep.CodeInternal, "exec: %w", err)
	}
	h.log.Info("exec finished", "event", "exec", "exit_code", res.ExitCode)
	return out, nil
}
