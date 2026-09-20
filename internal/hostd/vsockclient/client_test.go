package vsockclient

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
	fakeguestd "github.com/heracraft/repose/internal/hostd/fakeguestd"
)

func TestUnixDialerAgainstFake(t *testing.T) {
	dir := t.TempDir()
	srv, err := fakeguestd.Listen(filepath.Join(dir, "guestd.sock"), fakeguestd.Options{FreezeWatchdog: 200 * time.Millisecond, NeedsReboot: map[string]bool{"/nix/store/k": true}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := UnixDialer{}.Dial(ctx, Target{GuestID: "g", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	select {
	case n := <-s.Notifications():
		if n.GetReady() == nil {
			t.Fatalf("expected Ready, got %v", n)
		}
	case <-ctx.Done():
		t.Fatal("no Ready")
	}
	if p, err := s.Ping(ctx); err != nil || p.BootId != "boot-1" {
		t.Fatalf("ping: %v %v", p, err)
	}
	if err := s.WriteSecrets(ctx, []*guestdv1.Secret{{Name: "A", Value: []byte("1")}}); err != nil {
		t.Fatal(err)
	}
	if string(srv.Secrets()["A"]) != "1" {
		t.Fatal("secret not delivered")
	}
	sw, err := s.Switch(ctx, "/nix/store/k", false)
	if err != nil || !sw.NeedsReboot {
		t.Fatalf("switch: %v %v", sw, err)
	}
	if err := s.Freeze(ctx); err != nil {
		t.Fatal(err)
	}
	// Watchdog thaws and warns.
	select {
	case n := <-s.Notifications():
		if n.GetWarning().GetKind() != "freeze_timeout" {
			t.Fatalf("expected freeze_timeout, got %v", n)
		}
	case <-ctx.Done():
		t.Fatal("no watchdog warning")
	}
	if srv.Frozen() {
		t.Fatal("watchdog should have thawed")
	}
	if err := s.Shutdown(ctx, 5); err != nil {
		t.Fatal(err)
	}
	<-srv.ShutdownCalled()
	kinds := srv.Kinds()
	if kinds[0] != "Ping" || kinds[len(kinds)-1] != "Shutdown" {
		t.Fatalf("kinds: %v", kinds)
	}
}
