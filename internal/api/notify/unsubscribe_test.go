package notify_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/notify"
	"github.com/heracraft/repose/internal/db"
)

// fakePlatformSecrets is an in-memory stand-in for secrets.Store's
// GetPlatform/PutPlatform, enough to test key provisioning without a
// database.
type fakePlatformSecrets struct {
	mu     sync.Mutex
	values map[string][]byte
}

func newFakePlatformSecrets() *fakePlatformSecrets {
	return &fakePlatformSecrets{values: map[string][]byte{}}
}

func (f *fakePlatformSecrets) GetPlatform(_ context.Context, name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[name]
	if !ok {
		return nil, db.ErrNotFound
	}
	return v, nil
}

func (f *fakePlatformSecrets) PutPlatform(_ context.Context, name string, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[name] = value
	return nil
}

func TestLoadOrCreateUnsubscriberProvisionsOnce(t *testing.T) {
	sec := newFakePlatformSecrets()
	u1, err := notify.LoadOrCreateUnsubscriber(context.Background(), sec)
	if err != nil {
		t.Fatal(err)
	}
	u2, err := notify.LoadOrCreateUnsubscriber(context.Background(), sec)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if u1.Sign(id) != u2.Sign(id) {
		t.Fatal("second load minted a different key instead of reusing the stored one")
	}
	if len(sec.values) != 1 {
		t.Fatalf("expected exactly one stored secret, got %d", len(sec.values))
	}
}

func TestUnsubscriberVerifyRoundTrip(t *testing.T) {
	sec := newFakePlatformSecrets()
	u, err := notify.LoadOrCreateUnsubscriber(context.Background(), sec)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	token := u.Sign(id)
	got, err := u.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if got != id {
		t.Fatalf("verify returned %s, want %s", got, id)
	}
}

func TestUnsubscriberRejectsTamperedToken(t *testing.T) {
	sec := newFakePlatformSecrets()
	u, err := notify.LoadOrCreateUnsubscriber(context.Background(), sec)
	if err != nil {
		t.Fatal(err)
	}
	real := u.Sign(uuid.New())
	other := u.Sign(uuid.New())
	realID, _, _ := strings.Cut(real, ".")
	_, otherSig, _ := strings.Cut(other, ".")
	forged := realID + "." + otherSig
	if _, err := u.Verify(forged); err == nil {
		t.Fatal("a forged token verified")
	}
	if _, err := u.Verify("not-even-a-token"); err == nil {
		t.Fatal("a malformed token verified")
	}
	if _, err := u.Verify(""); err == nil {
		t.Fatal("an empty token verified")
	}
}

func TestUnsubscriberURL(t *testing.T) {
	sec := newFakePlatformSecrets()
	u, err := notify.LoadOrCreateUnsubscriber(context.Background(), sec)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	url := u.URL("https://api.repose.herakraft.co/", id)
	if !strings.HasPrefix(url, "https://api.repose.herakraft.co/v1/notify/unsubscribe?token=") {
		t.Fatalf("url %q", url)
	}
}
