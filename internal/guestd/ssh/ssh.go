// Package ssh implements the SetPrincipals request of
// docs/interfaces/vsock-guestd.md: the AuthorizedPrincipalsFile that decides
// which certificate may open this guest.
package ssh

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/heracraft/repose/internal/guestd/sysdep"
)

// MaxPrincipalBytes bounds one principal.
const MaxPrincipalBytes = 256

// MaxPrincipals bounds the list; a guest is reached by its project id and its
// slug.handle name, so the real number is two.
const MaxPrincipals = 64

// Handler writes the principals file.
type Handler struct {
	paths sysdep.Paths
	run   sysdep.Runner
	log   *slog.Logger
}

// New builds the handler.
func New(p sysdep.Paths, run sysdep.Runner, log *slog.Logger) *Handler {
	return &Handler{paths: p, run: run, log: log}
}

// Set replaces /etc/ssh/principals/dev and reloads sshd. hostd sends the same
// list at every start, so this is idempotent by design.
func (h *Handler) Set(ctx context.Context, principals []string) error {
	if len(principals) > MaxPrincipals {
		return sysdep.Invalid("set principals: %d principals, at most %d", len(principals), MaxPrincipals)
	}
	for _, p := range principals {
		switch {
		case strings.TrimSpace(p) == "":
			return sysdep.Invalid("set principals: a principal is empty")
		case len(p) > MaxPrincipalBytes:
			return sysdep.Invalid("set principals: a principal is longer than %d bytes", MaxPrincipalBytes)
		case strings.ContainsAny(p, "\n\r"):
			return sysdep.Invalid("set principals: a principal contains a newline")
		}
	}

	if err := os.MkdirAll(h.paths.PrincipalsDir(), 0o755); err != nil {
		return sysdep.Errf(sysdep.CodeInternal, "create principals directory: %w", err)
	}
	body := ""
	if len(principals) > 0 {
		body = strings.Join(principals, "\n") + "\n"
	}
	if err := sysdep.WriteFileAtomic(h.paths.Principals(), []byte(body), 0o644, 0, 0); err != nil {
		return sysdep.Errf(sysdep.CodeInternal, "write principals file: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	res, err := h.run.Run(ctx, sysdep.RunSpec{
		Argv:      []string{"systemctl", "reload-or-restart", "sshd.service"},
		MaxOutput: 4 << 10,
		Env:       sysdep.DevEnv(h.paths, "root"),
	})
	if err != nil {
		return sysdep.Errf(sysdep.CodeInternal, "reload sshd after writing principals: %w", err)
	}
	if res.ExitCode != 0 {
		return sysdep.Errf(sysdep.CodeInternal, "reload sshd after writing principals: systemctl exited %d", res.ExitCode)
	}

	// The principals themselves are project ids and slug.handle names; the
	// handle is on the never-log list, so only the count is logged.
	h.log.Info("principals written", "event", "set_principals", "count", len(principals))
	return nil
}
