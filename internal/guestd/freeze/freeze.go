// Package freeze implements the Freeze and Thaw requests of
// docs/interfaces/vsock-guestd.md, with the watchdog that keeps a hostd crash
// mid-snapshot from blocking every write in the guest forever.
package freeze

import (
	"log/slog"
	"sync"
	"time"

	"github.com/heracraft/repose/internal/guestd/sysdep"
)

// WarnFreezeTimeout is the Warning kind the watchdog sends.
const WarnFreezeTimeout = "freeze_timeout"

// DefaultTimeout is how long a freeze may last without a Thaw
// (docs/interfaces/vsock-guestd.md).
const DefaultTimeout = 10 * time.Second

// Warner receives the watchdog's warning. The server's notify queue implements
// it; it must not block.
type Warner func(kind, detail string)

// Handler owns the frozen state of the root filesystem. Exactly one exists.
type Handler struct {
	fz      sysdep.Freezer
	path    string
	timeout time.Duration
	warn    Warner
	log     *slog.Logger

	mu     sync.Mutex
	frozen bool
	timer  *time.Timer
	// timeouts counts watchdog firings, for the VM test and for metrics.
	timeouts int
}

// New builds the handler. timeout of zero means DefaultTimeout.
func New(fz sysdep.Freezer, path string, timeout time.Duration, warn Warner, log *slog.Logger) *Handler {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if warn == nil {
		warn = func(string, string) {}
	}
	return &Handler{fz: fz, path: path, timeout: timeout, warn: warn, log: log}
}

// Freeze freezes the root filesystem and arms the watchdog. A second Freeze
// while frozen re-arms the watchdog and succeeds, so a hostd that resent the
// command does not deadlock itself.
func (h *Handler) Freeze() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.frozen {
		h.rearmLocked()
		return nil
	}
	if err := h.fz.Freeze(h.path); err != nil {
		return sysdep.Errf(sysdep.CodeInternal, "freeze root filesystem: %w", err)
	}
	h.frozen = true
	h.rearmLocked()
	h.log.Info("root filesystem frozen", "event", "freeze", "timeout_ms", h.timeout.Milliseconds())
	return nil
}

// Thaw releases the filesystem and disarms the watchdog. Thawing an unfrozen
// filesystem succeeds; hostd retries are not errors.
func (h *Handler) Thaw() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.thawLocked("thaw")
}

func (h *Handler) thawLocked(event string) error {
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
	if !h.frozen {
		return nil
	}
	if err := h.fz.Thaw(h.path); err != nil {
		return sysdep.Errf(sysdep.CodeInternal, "thaw root filesystem: %w", err)
	}
	h.frozen = false
	h.log.Info("root filesystem thawed", "event", event)
	return nil
}

func (h *Handler) rearmLocked() {
	if h.timer != nil {
		h.timer.Stop()
	}
	h.timer = time.AfterFunc(h.timeout, h.fire)
}

// fire is the watchdog: no Thaw arrived, so guestd thaws itself and tells
// hostd, which fails the snapshot rather than leaving the guest wedged.
func (h *Handler) fire() {
	h.mu.Lock()
	if !h.frozen {
		h.mu.Unlock()
		return
	}
	h.timeouts++
	err := h.thawLocked(WarnFreezeTimeout)
	h.mu.Unlock()

	if err != nil {
		h.log.Error("watchdog could not thaw the root filesystem",
			"event", WarnFreezeTimeout, "error_code", sysdep.CodeOf(err))
		h.warn(WarnFreezeTimeout, "thaw after timeout failed")
		return
	}
	h.log.Warn("thawed by watchdog: no Thaw within the timeout",
		"event", WarnFreezeTimeout, "timeout_ms", h.timeout.Milliseconds())
	h.warn(WarnFreezeTimeout, "no Thaw within "+h.timeout.String()+"; filesystem thawed")
}

// Frozen reports whether the root filesystem is frozen right now.
func (h *Handler) Frozen() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.frozen
}

// Timeouts counts how often the watchdog has fired.
func (h *Handler) Timeouts() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.timeouts
}

// Close disarms the watchdog and thaws if still frozen, so a guestd shutdown
// never leaves the filesystem blocked.
func (h *Handler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.thawLocked("thaw")
}
