package guest

import (
	"context"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/hostd/vsockclient"
)

// A Switch that lost its connection retries on a new session, never on the
// one that just failed: until the monitor notices the drop, that dead
// session is still its current one (the TestBuildAndApply CI flake, whose
// retry failed with "vsockrpc: EOF").
func TestAwaitSessionSkipsTheSessionThatFailed(t *testing.T) {
	var dead, fresh vsockclient.Session = &fakeSess{}, &fakeSess{}
	mon := &monitor{sess: dead}
	m := &Manager{monitors: map[string]*monitor{"g": mon}, cfg: Config{GuestdRetry: 20 * time.Millisecond}}

	if _, err := m.awaitSession(context.Background(), "g", 60*time.Millisecond, dead); err == nil {
		t.Fatal("awaitSession handed back the session that had just failed")
	}
	go func() {
		time.Sleep(30 * time.Millisecond)
		mon.mu.Lock()
		mon.sess = fresh
		mon.mu.Unlock()
	}()
	s, err := m.awaitSession(context.Background(), "g", time.Second, dead)
	if err != nil || s != fresh {
		t.Fatalf("awaitSession = %v, %v; want the new session", s, err)
	}
}

// fakeSess is only compared by identity.
type fakeSess struct{ vsockclient.Session }
