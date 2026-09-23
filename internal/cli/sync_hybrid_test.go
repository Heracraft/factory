package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// I-203 against the local sshd harness, with "GitHub" played by the
// fixture's bare origin: a guest with no commits fetches the history
// itself and the laptop sends only its unpushed commit; a clone that
// fails falls back to the whole history with one line saying why.
func TestHybridFirstSync(t *testing.T) {
	for _, tc := range []struct {
		name      string
		url       func(f *syncFixture) string
		cloned    bool
		wantCount int
	}{
		{"clones", func(f *syncFixture) string { return f.bare }, true, 1},
		{"falls back", func(f *syncFixture) string { return filepath.Join(f.bare, "missing") }, false, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSyncFixture(t)
			if err := os.RemoveAll(f.guestRepo()); err != nil {
				t.Fatal(err)
			}
			prevURL, prevMin := hybridCloneURL, hybridThresholdKiB
			hybridCloneURL = func(string) string { return tc.url(f) }
			hybridThresholdKiB = 0
			t.Cleanup(func() { hybridCloneURL, hybridThresholdKiB = prevURL, prevMin })

			if err := os.WriteFile(filepath.Join(f.local, "README.md"), []byte("unpushed\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			mustRun(t, f.local, "git", "commit", "-q", "-am", "unpushed")
			s, err := syncGuest(context.Background(), f.target, f.local, testSlug, SyncOptions{RemoteURL: "github.com/a/b"})
			if err != nil {
				t.Fatalf("sync: %v", err)
			}
			t.Logf("%s / %v", s.String(), s.Warnings())
			if (s.ClonedFrom != "") != tc.cloned || s.Commits != tc.wantCount {
				t.Fatalf("summary = %+v", s)
			}
			if tc.cloned && !strings.Contains(s.String(), "history cloned from github.com") {
				t.Errorf("summary line = %q", s.String())
			}
			if !tc.cloned && (s.CloneFailed == "" || !strings.Contains(strings.Join(s.Warnings(), "\n"), "could not clone from GitHub")) {
				t.Errorf("fall back not said: %+v %v", s, s.Warnings())
			}
			if got, want := mustRun(t, f.guestRepo(), "git", "rev-parse", "HEAD"), mustRun(t, f.local, "git", "rev-parse", "HEAD"); got != want {
				t.Fatalf("guest HEAD %s, laptop %s", got, want)
			}
			// The next sync is a delta: no clone attempted.
			s, err = syncGuest(context.Background(), f.target, f.local, testSlug, SyncOptions{RemoteURL: "github.com/a/b"})
			if err != nil || s.ClonedFrom != "" || s.CloneFailed != "" || s.Commits != 0 {
				t.Fatalf("second sync = %+v %v", s, err)
			}
		})
	}
}

func TestHybridCloneURL(t *testing.T) {
	for remote, want := range map[string]string{
		"github.com/a/b":     "https://github.com/a/b.git",
		"github.com/a/b.git": "https://github.com/a/b.git",
		"gitlab.com/a/b":     "",
		"":                   "",
	} {
		if got := hybridCloneURL(remote); got != want {
			t.Errorf("hybridCloneURL(%q) = %q, want %q", remote, got, want)
		}
	}
}
