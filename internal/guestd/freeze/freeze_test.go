package freeze

import (
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/guestd/sysdep"
)

func quietLog() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

type warnRecorder struct {
	mu   sync.Mutex
	kind []string
}

func (w *warnRecorder) warn(kind, _ string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.kind = append(w.kind, kind)
}

func (w *warnRecorder) kinds() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, len(w.kind))
	copy(out, w.kind)
	return out
}

func TestFreezeThaw(t *testing.T) {
	fz := &sysdep.FakeFreezer{}
	h := New(fz, "/", time.Minute, nil, quietLog())

	if err := h.Freeze(); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if frozen, freezes, _ := fz.Frozen(); !frozen || freezes != 1 {
		t.Fatalf("after Freeze: frozen=%v freezes=%d", frozen, freezes)
	}
	if err := h.Thaw(); err != nil {
		t.Fatalf("thaw: %v", err)
	}
	if frozen, _, thaws := fz.Frozen(); frozen || thaws != 1 {
		t.Fatalf("after Thaw: frozen=%v thaws=%d", frozen, thaws)
	}
}

func TestFreezeIsIdempotent(t *testing.T) {
	fz := &sysdep.FakeFreezer{}
	h := New(fz, "/", time.Minute, nil, quietLog())

	for i := 0; i < 3; i++ {
		if err := h.Freeze(); err != nil {
			t.Fatalf("freeze %d: %v", i, err)
		}
	}
	// A resent Freeze re-arms the watchdog rather than freezing twice, which
	// would leave the filesystem frozen after a single Thaw.
	if _, freezes, _ := fz.Frozen(); freezes != 1 {
		t.Fatalf("freezes = %d, want 1", freezes)
	}
	if err := h.Thaw(); err != nil {
		t.Fatalf("thaw: %v", err)
	}
	if frozen, _, _ := fz.Frozen(); frozen {
		t.Fatal("still frozen after one Thaw")
	}
}

func TestThawWithoutFreezeSucceeds(t *testing.T) {
	fz := &sysdep.FakeFreezer{}
	h := New(fz, "/", time.Minute, nil, quietLog())
	if err := h.Thaw(); err != nil {
		t.Fatalf("thaw: %v", err)
	}
	if _, _, thaws := fz.Frozen(); thaws != 0 {
		t.Fatalf("thaws = %d, want 0: an unfrozen filesystem is not thawed again", thaws)
	}
}

func TestWatchdogThawsAndWarns(t *testing.T) {
	fz := &sysdep.FakeFreezer{}
	rec := &warnRecorder{}
	h := New(fz, "/", 50*time.Millisecond, rec.warn, quietLog())

	if err := h.Freeze(); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if frozen, _, _ := fz.Frozen(); !frozen {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if frozen, _, thaws := fz.Frozen(); frozen || thaws != 1 {
		t.Fatalf("watchdog did not thaw: frozen=%v thaws=%d", frozen, thaws)
	}
	if got := rec.kinds(); len(got) != 1 || got[0] != WarnFreezeTimeout {
		t.Fatalf("warnings = %v, want one %s", got, WarnFreezeTimeout)
	}
	if h.Timeouts() != 1 {
		t.Fatalf("timeouts = %d, want 1", h.Timeouts())
	}
}

func TestWatchdogDoesNotFireAfterThaw(t *testing.T) {
	fz := &sysdep.FakeFreezer{}
	rec := &warnRecorder{}
	h := New(fz, "/", 100*time.Millisecond, rec.warn, quietLog())

	if err := h.Freeze(); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if err := h.Thaw(); err != nil {
		t.Fatalf("thaw: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if got := rec.kinds(); len(got) != 0 {
		t.Fatalf("warnings = %v, want none", got)
	}
}

func TestCloseThaws(t *testing.T) {
	fz := &sysdep.FakeFreezer{}
	h := New(fz, "/", time.Minute, nil, quietLog())
	if err := h.Freeze(); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if err := h.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if frozen, _, _ := fz.Frozen(); frozen {
		t.Fatal("Close left the filesystem frozen")
	}
}
