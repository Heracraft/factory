package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// I-210 edge cases found by the second review of ws/15-fixes.

func wantDirtyRefusal(t *testing.T, err error) {
	t.Helper()
	ee, ok := err.(*exitError)
	if !ok || ee.code != ExitDirtyRemoteTree {
		t.Fatalf("err = %v, want exit %d", err, ExitDirtyRemoteTree)
	}
}

// An agent's edit inside a submodule does not change the superproject's
// `git add -A` tree (a submodule is only its commit there), and a carried
// submodule.recurse=true would make a reset restore the file. Any
// submodule change means the tree is not the last sync's own.
func TestSyncedTreeWithASubmoduleChangeRefuses(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	subBare := t.TempDir()
	mustRun(t, subBare, "git", "init", "--bare", "-q", "-b", "main", ".")
	seed := t.TempDir()
	mustRun(t, seed, "git", "clone", "-q", subBare, ".")
	if err := os.WriteFile(filepath.Join(seed, "s"), []byte("sub v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, seed, "git", "add", "s")
	mustRun(t, seed, "git", "-c", "user.email=s@x", "-c", "user.name=s", "commit", "-q", "-m", "sub")
	mustRun(t, seed, "git", "push", "-q", "origin", "main")
	mustRun(t, f.local, "git", "-c", "protocol.file.allow=always", "submodule", "add", "-q", subBare, "sub")
	mustRun(t, f.local, "git", "commit", "-q", "-m", "add sub")

	if err := os.WriteFile(filepath.Join(f.local, "README.md"), []byte("laptop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := syncGuest(ctx, f.target, f.local, testSlug, SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	mustRun(t, f.guestRepo(), "git", "-c", "protocol.file.allow=always", "submodule", "update", "-q", "--init")
	mustRun(t, f.guestRepo(), "git", "config", "submodule.recurse", "true")
	agent := filepath.Join(f.guestRepo(), "sub", "s")
	if err := os.WriteFile(agent, []byte("the agent's work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := syncGuest(ctx, f.target, f.local, testSlug, SyncOptions{})
	wantDirtyRefusal(t, err)
	if b, _ := os.ReadFile(agent); string(b) != "the agent's work\n" {
		t.Fatalf("the agent's edit in the submodule was lost: %q", b)
	}
}

// A carried status.showUntrackedFiles=no hides untracked files from a
// plain `git status`: a sync that left only untracked files must still
// fingerprint them, and the agent's edit of one must still refuse.
func TestSyncedTreeIgnoresShowUntrackedFilesNo(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	mustRun(t, f.guestRepo(), "git", "config", "status.showUntrackedFiles", "no")
	if err := os.WriteFile(filepath.Join(f.local, "notes.md"), []byte("laptop notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := syncGuest(ctx, f.target, f.local, testSlug, SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	// The same laptop tree again goes through.
	if _, err := syncGuest(ctx, f.target, f.local, testSlug, SyncOptions{}); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	agent := filepath.Join(f.guestRepo(), "notes.md")
	if err := os.WriteFile(agent, []byte("the agent's notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := syncGuest(ctx, f.target, f.local, testSlug, SyncOptions{})
	wantDirtyRefusal(t, err)
	if b, _ := os.ReadFile(agent); string(b) != "the agent's notes\n" {
		t.Fatalf("the agent's notes were overwritten: %q", b)
	}
}

// The last sync's changes are stashed, not reset away, so anything
// misjudged can be recovered, and the summary says so. --stash-remote
// and --discard-remote keep their meaning.
func TestSyncedTreeIsStashed(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(f.local, "README.md"), []byte("laptop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := syncGuest(ctx, f.target, f.local, testSlug, SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	s, err := syncGuest(ctx, f.target, f.local, testSlug, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if list := mustRun(t, f.guestRepo(), "git", "stash", "list"); !strings.Contains(list, "repose run: last sync") {
		t.Fatalf("stash list = %q", list)
	}
	if !strings.Contains(s.String(), "last sync's changes stashed in the guest") {
		t.Errorf("summary = %q", s.String())
	}
	if b, _ := os.ReadFile(filepath.Join(f.guestRepo(), "README.md")); string(b) != "laptop\n" {
		t.Errorf("README.md = %q", b)
	}
	stashes := mustRun(t, f.guestRepo(), "git", "stash", "list")
	s, err = syncGuest(ctx, f.target, f.local, testSlug, SyncOptions{DiscardRemote: true})
	if err != nil || s.StashedLastSync {
		t.Fatalf("--discard-remote: %+v %v", s, err)
	}
	if after := mustRun(t, f.guestRepo(), "git", "stash", "list"); after != stashes {
		t.Errorf("--discard-remote stashed: %q, before %q", after, stashes)
	}
}

// The last-sync stashes are capped at the newest ten; the user's own
// stashes (and --stash-remote's "repose run") are never dropped.
func TestSyncedStashesAreCapped(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(f.guestRepo(), "README.md"), []byte("the user's own work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, f.guestRepo(), "git", "stash", "push", "-q", "-m", "my work")
	for i := 0; i < 13; i++ { // the first leaves the tree dirty; the next 12 stash
		if err := os.WriteFile(filepath.Join(f.local, "README.md"), []byte(strings.Repeat("x", i+1)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := syncGuest(ctx, f.target, f.local, testSlug, SyncOptions{}); err != nil {
			t.Fatalf("sync %d: %v", i, err)
		}
	}
	list := mustRun(t, f.guestRepo(), "git", "stash", "list", "--format=%gs")
	if n := strings.Count(list, ": repose run: last sync"); n != syncStashKeep {
		t.Errorf("%d last-sync stashes, want %d:\n%s", n, syncStashKeep, list)
	}
	if !strings.Contains(list, ": my work") {
		t.Errorf("the user's stash is gone:\n%s", list)
	}
	// The newest are the ones kept: the top stash holds run 12's tree.
	if b := mustRun(t, f.guestRepo(), "git", "show", "stash@{0}:README.md"); b != strings.Repeat("x", 12) {
		t.Errorf("stash@{0} README.md = %q", b)
	}
}

// A refused run leaves nothing behind in the guest's object store: the
// probe's fingerprint hashes the agent's files without writing them.
func TestSyncedProbeWritesNoObjects(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(f.local, "README.md"), []byte("laptop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := syncGuest(ctx, f.target, f.local, testSlug, SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.guestRepo(), "agent.txt"), []byte("brand new content 8d1f\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := mustRun(t, f.guestRepo(), "git", "count-objects")
	_, err := syncGuest(ctx, f.target, f.local, testSlug, SyncOptions{})
	wantDirtyRefusal(t, err)
	if after := mustRun(t, f.guestRepo(), "git", "count-objects"); after != before {
		t.Errorf("objects before %q, after the refused run %q", before, after)
	}
}
