package guest

import (
	"context"
	"sync"
	"testing"
	"time"

	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
)

// TestStopKeepsConsoleUntilTheHypervisorExits pins I-186: a stop ended
// console capture right after asking guestd to shut down, while the guest
// was still printing its shutdown. Closing the console socket with bytes
// unread killed Cloud Hypervisor's serial thread, the guest's console
// stalled and PID 1 with it, and the stop waited out its whole timeout
// (e2e-a-private on host-01, 3 of 3 stops). Capture now ends only once the
// guest unit is inactive.
func TestStopKeepsConsoleUntilTheHypervisorExits(t *testing.T) {
	h := newHarness(t, nil)
	var mu sync.Mutex
	var log []string
	unit := GuestUnit(gid1)
	h.m.d.ConsoleStart = func(guestID, dir string) func() {
		mu.Lock()
		log = append(log, "console start")
		mu.Unlock()
		return func() {
			active, _ := h.sd.IsActive(context.Background(), unit)
			mu.Lock()
			if active {
				log = append(log, "console stop while the hypervisor runs")
			} else {
				log = append(log, "console stop after exit")
			}
			mu.Unlock()
		}
	}
	h.create(gid1)
	// The guest takes a moment to power off, as a real one does.
	h.guestd(gid1).OnShutdown(func() {
		go func() {
			time.Sleep(300 * time.Millisecond)
			h.sd.Exit(unit, 0)
		}()
	})
	h.mustOK(cmd(&hostdv1.StopGuest{GuestId: gid1, TimeoutS: 5}))
	mu.Lock()
	defer mu.Unlock()
	if len(log) != 2 || log[1] != "console stop after exit" {
		t.Fatalf("console capture: %v", log)
	}
}
