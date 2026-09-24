package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
)

// I-248: `repose run` on a machine that changed since the last sync.

// syncedWithALaptopEdit is a fixture after one sync that carried a laptop
// edit, so the guest holds the last sync's key.
func syncedWithALaptopEdit(t *testing.T) *syncFixture {
	t.Helper()
	f := newSyncFixture(t)
	if err := os.WriteFile(filepath.Join(f.local, "README.md"), []byte("laptop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := syncGuest(context.Background(), f.target, f.local, testSlug, SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	return f
}

// The laptop has nothing new: an agent's uncommitted files are left as
// they are, the run goes on, and the summary says so in one line. Only
// the probe's ssh runs.
func TestSyncLeavesTheGuestAloneWhenTheLaptopHasNothingNew(t *testing.T) {
	f := syncedWithALaptopEdit(t)
	agent := map[string]string{"README.md": "the agent's README\n"}
	for i := 0; i < 26; i++ {
		agent[fmt.Sprintf("gen/f%02d.go", i)] = fmt.Sprintf("package gen // %d\n", i)
	}
	for rel, body := range agent {
		p := filepath.Join(f.guestRepo(), rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var s *SyncSummary
	n := countSSH(t, func() {
		var err error
		if s, err = syncGuest(context.Background(), f.target, f.local, testSlug, SyncOptions{}); err != nil {
			t.Fatalf("sync with nothing new on the laptop: %v", err)
		}
	})
	if !s.GuestAhead || s.Unchanged || n != 1 {
		t.Fatalf("guestAhead=%v unchanged=%v ssh=%d", s.GuestAhead, s.Unchanged, n)
	}
	// git status names the untracked directory once: README.md and gen/.
	want := "The machine has changes your laptop doesn't have (2 files); attaching without syncing. `repose run --stash-remote` puts them in git stash and syncs your laptop's work."
	if s.String() != want {
		t.Fatalf("summary = %q\nwant      %q", s.String(), want)
	}
	for rel, body := range agent {
		if b, _ := os.ReadFile(filepath.Join(f.guestRepo(), rel)); string(b) != body {
			t.Fatalf("%s touched: %q", rel, b)
		}
	}
	if list := mustRun(t, f.guestRepo(), "git", "stash", "list"); list != "" {
		t.Fatalf("stashed: %q", list)
	}

	// --stash-remote still syncs over them, keeping them in the stash.
	if s, err := syncGuest(context.Background(), f.target, f.local, testSlug, SyncOptions{StashRemote: true}); err != nil || s.GuestAhead {
		t.Fatalf("--stash-remote: %+v %v", s, err)
	}
	if b, _ := os.ReadFile(filepath.Join(f.guestRepo(), "README.md")); string(b) != "laptop\n" {
		t.Fatalf("README.md after --stash-remote = %q", b)
	}
	if list := mustRun(t, f.guestRepo(), "git", "stash", "list"); !strings.Contains(list, "repose run") {
		t.Fatalf("stash list = %q", list)
	}
}

// An agent that committed on the branch, with nothing new on the laptop:
// the guest stays on its branch (not checked out detached at the laptop's
// older commit), and the line says what is there.
func TestSyncLeavesTheGuestsCommitsAlone(t *testing.T) {
	f := newSyncFixture(t)
	if _, err := syncGuest(context.Background(), f.target, f.local, testSlug, SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.guestRepo(), "feature.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, f.guestRepo(), "git", "add", "feature.go")
	mustRun(t, f.guestRepo(), "git", "-c", "user.email=a@x", "-c", "user.name=agent", "commit", "-q", "-m", "agent's commit")
	agentHead := mustRun(t, f.guestRepo(), "git", "rev-parse", "HEAD")

	s, err := syncGuest(context.Background(), f.target, f.local, testSlug, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !s.GuestAhead || s.Detached || !strings.HasPrefix(s.String(), "The machine has commits your laptop doesn't have; attaching without syncing.") {
		t.Fatalf("summary %+v %q", s, s.String())
	}
	if got := mustRun(t, f.guestRepo(), "git", "rev-parse", "HEAD"); got != agentHead {
		t.Fatalf("guest HEAD moved to %s", got)
	}
	if ref := mustRun(t, f.guestRepo(), "git", "symbolic-ref", "-q", "HEAD"); ref == "" {
		t.Fatal("guest detached")
	}
}

// With new laptop work, the refusal says what `run` does, what is on the
// machine (eight names, then a count) and the three ways on.
func TestSyncRefusalSaysWhatRunDoes(t *testing.T) {
	f := syncedWithALaptopEdit(t)
	for i := 0; i < 27; i++ {
		if err := os.WriteFile(filepath.Join(f.guestRepo(), fmt.Sprintf("agent%02d.txt", i)), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(f.local, "README.md"), []byte("laptop, later\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := syncGuest(context.Background(), f.target, f.local, testSlug, SyncOptions{})
	wantDirtyRefusal(t, err)
	msg := err.(*exitError).msg
	want := "`repose run` copies your laptop's work onto the machine. It doesn't restart or rebuild anything.\n" +
		"The machine has uncommitted changes your laptop doesn't have (28 files), probably an agent's:\n" +
		"  README.md\n  agent00.txt\n  agent01.txt\n  agent02.txt\n  agent03.txt\n  agent04.txt\n  agent05.txt\n  agent06.txt\n" +
		"  and 20 more\n" +
		"Your laptop has new work as well, so syncing now would write over them. Nothing was changed. Pick one:\n" +
		"  repose attach                  look at the machine first\n" +
		"  repose run --stash-remote      put the machine's changes in git stash, then sync\n" +
		"  repose run --discard-remote    throw the machine's changes away, then sync"
	if msg != want {
		t.Fatalf("message:\n%s\nwant:\n%s", msg, want)
	}
	if b, _ := os.ReadFile(filepath.Join(f.guestRepo(), "README.md")); string(b) != "laptop\n" {
		t.Fatalf("README.md changed by a refused run: %q", b)
	}
}

func TestDirtyTreeErrorShortListHasNoCount(t *testing.T) {
	msg := (&dirtyTreeError{files: []string{" M a.go", "?? b/", "M  c.go"}}).Error()
	if !strings.Contains(msg, "(3 files)") || !strings.Contains(msg, "\n  a.go\n  b/\n  c.go\n") || strings.Contains(msg, "more") {
		t.Fatalf("message:\n%s", msg)
	}
	nine := make([]string, 9)
	for i := range nine {
		nine[i] = fmt.Sprintf("?? f%d", i)
	}
	if msg := (&dirtyTreeError{files: nine}).Error(); strings.Contains(msg, "more") || !strings.Contains(msg, "  f8\n") {
		t.Fatalf("nine files are listed in full:\n%s", msg)
	}
}

// End to end through runRun: the second run, from an unchanged laptop,
// goes through and prints the one line.
func TestRunAttachesWhenOnlyTheMachineChanged(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()
	f := newRunFixture(t, fake)
	ctx := context.Background()
	if err := runRun(ctx, f.env, RunOptions{Name: testSlug, NoAttach: true}, false); err != nil {
		t.Fatalf("first runRun: %v", err)
	}
	if err := os.WriteFile(filepath.Join(f.guestRepo(), "README.md"), []byte("agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	env2 := &Env{
		Dir: f.env.Dir, Cfg: f.env.Cfg, Cache: f.env.Cache, Cwd: f.local, HomeDir: f.env.HomeDir,
		Client: f.env.Client, Out: &out, ErrOut: &discardWriter{}, TargetFor: f.env.TargetFor,
	}
	if err := runRun(ctx, env2, RunOptions{NoAttach: true}, false); err != nil {
		t.Fatalf("second runRun: %v", err)
	}
	if !strings.Contains(out.String(), "The machine has changes your laptop doesn't have (1 file); attaching without syncing.") {
		t.Fatalf("output = %q", out.String())
	}
	if b, _ := os.ReadFile(filepath.Join(f.guestRepo(), "README.md")); string(b) != "agent\n" {
		t.Fatalf("README.md = %q", b)
	}
}
