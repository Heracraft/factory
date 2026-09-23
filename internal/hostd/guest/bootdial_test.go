package guest

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/hostd/fakeguestd"
)

// I-225: until its first guestd session a monitor dials every
// GuestdBootRetry, not every GuestdRetry, so a create (or start) is
// marked running within a fraction of a second of guestd listening, not
// up to GuestdRetry later.
func TestBootDialFindsGuestdSoon(t *testing.T) {
	for _, c := range []struct {
		boot    time.Duration
		maxTook time.Duration
	}{
		{0, 1500 * time.Millisecond},               // the default, 200 ms
		{2 * time.Second, 2500 * time.Millisecond}, // what every start paid before
	} {
		h := newHarness(t, func(cfg *Config) {
			cfg.GuestdRetry = 2 * time.Second
			cfg.GuestdBootRetry = c.boot
			cfg.ReadyTimeout = 10 * time.Second
		})
		const id = "01a0cfac-851e-7d29-83d0-2bad2804cb21"
		h.mu.Lock()
		h.noBoot[id] = true
		h.mu.Unlock()
		// guestd starts listening 300 ms into the boot.
		go func() {
			time.Sleep(300 * time.Millisecond)
			srv, err := fakeguestd.Listen(filepath.Join(h.sockDir, id+".sock"), h.gopts)
			if err != nil {
				t.Error(err)
				return
			}
			h.mu.Lock()
			h.guestds[id] = srv
			h.mu.Unlock()
		}()
		start := time.Now()
		h.create(id)
		took := time.Since(start)
		if c.boot == 0 && took > c.maxTook {
			t.Fatalf("create took %s with the boot retry; guestd listened after 300ms", took)
		}
		if c.boot != 0 && took < 1500*time.Millisecond {
			t.Fatalf("create took %s with a 2 s boot retry: the test does not measure the retry", took)
		}
		t.Logf("boot retry %v: create took %s", c.boot, took.Round(10*time.Millisecond))
	}
}
