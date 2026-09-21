package control

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/hostd/guest"
)

type fakeBackend struct{ logins []OperatorLoginReport }

func (f *fakeBackend) Status(context.Context) (*StatusReply, error) {
	return &StatusReply{HostID: "h"}, nil
}
func (f *fakeBackend) Guests(context.Context) ([]guest.Status, error) { return nil, nil }
func (f *fakeBackend) SnapshotAll(string) ([]string, error)           { return nil, nil }
func (f *fakeBackend) Drain(bool) error                               { return nil }
func (f *fakeBackend) Reconcile(context.Context, bool) ([]string, error) {
	return nil, nil
}
func (f *fakeBackend) ExportState(io.Writer) error { return nil }
func (f *fakeBackend) OperatorLogin(r OperatorLoginReport) error {
	f.logins = append(f.logins, r)
	return nil
}

// `hostd audit-login` hands an SSH login to the daemon over the control
// socket; the daemon turns it into the host Event the api audits (I-140).
func TestOperatorLoginRoundTrip(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "hostd.sock")
	b := &fakeBackend{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = Serve(ctx, sock, b) }()
	c := NewClient(sock)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := c.Status(ctx); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("control socket never answered")
		}
		time.Sleep(20 * time.Millisecond)
	}
	rep := OperatorLoginReport{PAMType: "open_session", UserPresent: true, KeyID: "operator:alice", Serial: 7, KeyFingerprint: "SHA256:abc"}
	if err := c.OperatorLogin(ctx, rep); err != nil {
		t.Fatal(err)
	}
	if len(b.logins) != 1 || b.logins[0] != rep {
		t.Fatalf("backend saw %+v", b.logins)
	}
	// Without a daemon the hook must fail fast and quietly; the journal
	// line is the record then.
	if err := NewClient(filepath.Join(t.TempDir(), "absent.sock")).OperatorLogin(ctx, rep); err != ErrNotRunning {
		t.Fatalf("absent socket: %v", err)
	}
}
