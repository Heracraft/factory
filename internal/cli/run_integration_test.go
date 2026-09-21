package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/testguest"
)

// This file is the integration coverage docs/workstreams/07-cli.md §7 asks
// for ("full run sequence with a local sshd ... standing in for the
// guest"); see internal/testguest's doc comment for why that sshd is an
// in-process Go server instead of the Docker fixture the doc names.

const testSlug = "proj"

func mustRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s (in %s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// syncFixture is a bare "origin", a local clone (what the CLI runs
// against) and a fake guest whose $HOME/<slug> is a second clone of the
// same commit, connected over a real SSH session to an in-process sshd.
type syncFixture struct {
	bare, local, guestHome string
	guest                  *testguest.Guest
	target                 sshTarget
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	bare := t.TempDir()
	mustRun(t, bare, "git", "init", "--bare", "-q", "-b", "main", ".")

	seed := t.TempDir()
	mustRun(t, seed, "git", "clone", "-q", bare, ".")
	mustRun(t, seed, "git", "config", "user.email", "seed@example.com")
	mustRun(t, seed, "git", "config", "user.name", "Seed")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, seed, "git", "add", "README.md")
	mustRun(t, seed, "git", "commit", "-q", "-m", "initial")
	mustRun(t, seed, "git", "push", "-q", "origin", "main")

	local := t.TempDir()
	mustRun(t, local, "git", "clone", "-q", bare, ".")
	mustRun(t, local, "git", "config", "user.email", "dev@example.com")
	mustRun(t, local, "git", "config", "user.name", "Dev Laptop")

	guestHome := t.TempDir()
	guestRepo := filepath.Join(guestHome, testSlug)
	mustRun(t, guestHome, "git", "clone", "-q", bare, testSlug)
	mustRun(t, guestRepo, "git", "config", "user.email", "guest@example.com")
	mustRun(t, guestRepo, "git", "config", "user.name", "Guest")

	keyDir := t.TempDir()
	privPath, pub, err := testguest.GenerateClientKey(keyDir)
	if err != nil {
		t.Fatal(err)
	}
	guest, err := testguest.New(guestHome, pub)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(guest.Close)

	host, port, _ := strings.Cut(guest.Addr, ":")
	target := sshTarget{Args: []string{
		"-p", port,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "IdentitiesOnly=yes",
		"-o", "BatchMode=yes",
		"-i", privPath,
		"guest@" + host,
	}}

	return &syncFixture{bare: bare, local: local, guestHome: guestHome, guest: guest, target: target}
}

func (f *syncFixture) guestRepo() string { return filepath.Join(f.guestHome, testSlug) }

func TestSyncDirtyRemoteRefused(t *testing.T) {
	f := newSyncFixture(t)
	if err := os.WriteFile(filepath.Join(f.guestRepo(), "README.md"), []byte("agent was here\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := syncGuest(context.Background(), f.target, f.local, testSlug, SyncOptions{})
	ee, ok := err.(*exitError)
	if !ok {
		t.Fatalf("want *exitError, got %T: %v", err, err)
	}
	if ee.code != ExitDirtyRemoteTree {
		t.Fatalf("code = %d, want %d", ee.code, ExitDirtyRemoteTree)
	}
	if !strings.Contains(ee.msg, "README.md") {
		t.Fatalf("message missing the dirty file: %s", ee.msg)
	}
}

func TestSyncStashRemote(t *testing.T) {
	f := newSyncFixture(t)
	if err := os.WriteFile(filepath.Join(f.guestRepo(), "README.md"), []byte("agent was here\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := syncGuest(context.Background(), f.target, f.local, testSlug, SyncOptions{StashRemote: true}); err != nil {
		t.Fatalf("syncGuest: %v", err)
	}
	stashList := mustRun(t, f.guestRepo(), "git", "stash", "list")
	if stashList == "" {
		t.Fatal("expected a stash entry, got none")
	}
}

func TestSyncDiscardRemote(t *testing.T) {
	f := newSyncFixture(t)
	if err := os.WriteFile(filepath.Join(f.guestRepo(), "README.md"), []byte("agent was here\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := syncGuest(context.Background(), f.target, f.local, testSlug, SyncOptions{DiscardRemote: true}); err != nil {
		t.Fatalf("syncGuest: %v", err)
	}
	status := mustRun(t, f.guestRepo(), "git", "status", "--porcelain")
	if status != "" {
		t.Fatalf("guest tree still dirty after discard: %q", status)
	}
	content, err := os.ReadFile(filepath.Join(f.guestRepo(), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello\n" {
		t.Fatalf("discard did not revert README.md: %q", content)
	}
}

func TestSyncPushesUnpushedCommit(t *testing.T) {
	f := newSyncFixture(t)
	if err := os.WriteFile(filepath.Join(f.local, "README.md"), []byte("local change, not yet pushed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, f.local, "git", "add", "README.md")
	mustRun(t, f.local, "git", "commit", "-q", "-m", "local only")
	head := mustRun(t, f.local, "git", "rev-parse", "HEAD")

	var askedCommit, askedBranch string
	opts := SyncOptions{AskPush: func(commit, branch string) (bool, error) {
		askedCommit, askedBranch = commit, branch
		mustRun(t, f.local, "git", "push", "origin", branch)
		return true, nil
	}}
	if _, err := syncGuest(context.Background(), f.target, f.local, testSlug, opts); err != nil {
		t.Fatalf("syncGuest: %v", err)
	}
	if askedCommit != head {
		t.Fatalf("AskPush commit = %q, want %q", askedCommit, head)
	}
	if askedBranch != "main" {
		t.Fatalf("AskPush branch = %q, want main", askedBranch)
	}
	guestHead := mustRun(t, f.guestRepo(), "git", "rev-parse", "HEAD")
	if guestHead != head {
		t.Fatalf("guest HEAD = %s, want %s", guestHead, head)
	}
}

func TestSyncAppliesDiffAndUntracked(t *testing.T) {
	f := newSyncFixture(t)
	if err := os.WriteFile(filepath.Join(f.local, "README.md"), []byte("edited locally, uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.local, "notes.md"), []byte("scratch notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	summary, err := syncGuest(context.Background(), f.target, f.local, testSlug, SyncOptions{})
	if err != nil {
		t.Fatalf("syncGuest: %v", err)
	}
	if summary.Modified != 1 || summary.Untracked != 1 {
		t.Fatalf("summary = %+v, want 1 modified, 1 untracked", summary)
	}
	readme, err := os.ReadFile(filepath.Join(f.guestRepo(), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(readme) != "edited locally, uncommitted\n" {
		t.Fatalf("diff not applied: %q", readme)
	}
	notes, err := os.ReadFile(filepath.Join(f.guestRepo(), "notes.md"))
	if err != nil {
		t.Fatalf("untracked file not copied: %v", err)
	}
	if string(notes) != "scratch notes\n" {
		t.Fatalf("untracked content wrong: %q", notes)
	}
}

// A --name project has no remote, so the guest's repository has no origin
// (guestd sets one only from the project's remote_url, I-107). Before the
// M5 review the sync ran `git fetch origin` regardless and `repose run`
// failed after the guest had booted (security/review-2026-09-21.md M5-9).
func TestSyncNoRemoteSendsTheWholeTree(t *testing.T) {
	f := newSyncFixture(t)
	local := t.TempDir()
	mustRun(t, local, "git", "init", "-q", "-b", "main", ".")
	mustRun(t, local, "git", "config", "user.email", "dev@example.com")
	mustRun(t, local, "git", "config", "user.name", "Dev Laptop")
	if err := os.WriteFile(filepath.Join(local, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, local, "git", "add", "main.go")
	mustRun(t, local, "git", "commit", "-q", "-m", "local only")
	if err := os.WriteFile(filepath.Join(local, "main.go"), []byte("package main // edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "notes.md"), []byte("scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The guest's side is what guestd's SetupProject leaves for a project
	// without a remote: `git init`, no origin, no commit.
	guestRepo := f.guestRepo()
	if err := os.RemoveAll(guestRepo); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(guestRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	mustRun(t, guestRepo, "git", "init", "-q", "-b", "main", ".")

	summary, err := syncGuest(context.Background(), f.target, local, testSlug, SyncOptions{NoRemote: true})
	if err != nil {
		t.Fatalf("syncGuest: %v", err)
	}
	if !summary.WholeTree || summary.Tracked != 1 || summary.Untracked != 1 || summary.Modified != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if got := summary.String(); !strings.Contains(got, "no git remote") {
		t.Fatalf("summary line %q does not say why", got)
	}
	body, err := os.ReadFile(filepath.Join(guestRepo, "main.go"))
	if err != nil || string(body) != "package main // edited\n" {
		t.Fatalf("tracked file with its uncommitted edit not in the guest: %q %v", body, err)
	}
	if committed := mustRun(t, guestRepo, "git", "ls-tree", "--name-only", "HEAD"); committed != "main.go" {
		t.Fatalf("committed in the guest: %q, want main.go", committed)
	}
	if notes, err := os.ReadFile(filepath.Join(guestRepo, "notes.md")); err != nil || string(notes) != "scratch\n" {
		t.Fatalf("untracked file: %q %v", notes, err)
	}
	// The tracked tree is committed, so the guest is clean for the next
	// run's dirty check and an unchanged tree makes no second commit; the
	// untracked file counts as dirty exactly as it does for a project with
	// a remote, so it is removed before the second run.
	if err := os.Remove(filepath.Join(guestRepo, "notes.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(local, "notes.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := syncGuest(context.Background(), f.target, local, testSlug, SyncOptions{NoRemote: true}); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if n := mustRun(t, guestRepo, "git", "rev-list", "--count", "HEAD"); n != "1" {
		t.Fatalf("an unchanged tree made a second commit: %s", n)
	}
}

func TestSyncCredentialsCopiesExactlyTheFourRows(t *testing.T) {
	f := newSyncFixture(t)
	home := t.TempDir()

	write := func(rel, content string) {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(".config/gh/hosts.yml", "github.com:\n  oauth_token: abc\n")
	write(".codex/auth.json", `{"token":"abc"}`)
	write(".local/share/opencode/auth.json", `{"token":"abc"}`)
	// Planted but must never be copied.
	write(".claude/.credentials.json", `{"token":"should-never-travel"}`)
	write(".gemini/oauth_creds.json", `{"token":"should-never-travel"}`)
	write(".ssh/id_ed25519", "not a real key, must never travel")

	mustRun(t, f.local, "git", "config", "user.name", "Dev Laptop")
	mustRun(t, f.local, "git", "config", "user.email", "dev@example.com")

	copied, err := syncCredentials(context.Background(), f.target, home, f.local)
	if err != nil {
		t.Fatalf("syncCredentials: %v", err)
	}
	want := map[string]bool{"gh": true, "codex": true, "opencode": true, "git": true}
	if len(copied) != len(want) {
		t.Fatalf("copied = %v, want exactly %v", copied, want)
	}
	for _, c := range copied {
		if !want[c] {
			t.Fatalf("unexpected label copied: %s", c)
		}
	}

	for _, never := range []string{".claude/.credentials.json", ".gemini/oauth_creds.json", ".ssh/id_ed25519"} {
		if _, err := os.Stat(filepath.Join(f.guestHome, never)); err == nil {
			t.Fatalf("%s must never be synced but was found on the guest", never)
		}
	}
	ghBytes, err := os.ReadFile(filepath.Join(f.guestHome, ".config", "gh", "hosts.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ghBytes), "abc") {
		t.Fatalf("gh hosts.yml content wrong: %s", ghBytes)
	}
	info, err := os.Stat(filepath.Join(f.guestHome, ".codex", "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("codex auth.json mode = %v, want 0600", info.Mode().Perm())
	}
	gitconfig, err := os.ReadFile(filepath.Join(f.guestHome, ".gitconfig"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gitconfig), "Dev Laptop") || !strings.Contains(string(gitconfig), "dev@example.com") {
		t.Fatalf(".gitconfig missing identity: %s", gitconfig)
	}
}

func TestPromptSendAndSecondWindowNaming(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()

	if _, err := runSSH(ctx, f.target, "tmux new-session -d -s "+testSlug+" -c ~/"+testSlug, nil); err != nil {
		t.Fatalf("tmux new-session: %v", err)
	}

	name1, existed1, err := windowNameFor(ctx, f.target, testSlug, "cat")
	if err != nil {
		t.Fatal(err)
	}
	if name1 != "cat" || existed1 {
		t.Fatalf("first window name = %q existed=%v, want cat/false", name1, existed1)
	}
	if err := startAgentWindow(ctx, f.target, testSlug, name1, "cat", "hello agent", false); err != nil {
		t.Fatalf("startAgentWindow: %v", err)
	}
	pane, err := waitForCapture(ctx, f.target, testSlug, name1, "hello agent")
	if err != nil {
		t.Fatalf("prompt never appeared in the pane: %v\nlast capture:\n%s", err, pane)
	}

	name2, existed2, err := windowNameFor(ctx, f.target, testSlug, "cat")
	if err != nil {
		t.Fatal(err)
	}
	if name2 != "cat-2" || !existed2 {
		t.Fatalf("second window name = %q existed=%v, want cat-2/true", name2, existed2)
	}
	if err := startAgentWindow(ctx, f.target, testSlug, name2, "cat", "second prompt", false); err != nil {
		t.Fatalf("startAgentWindow (second): %v", err)
	}
	if _, err := waitForCapture(ctx, f.target, testSlug, name2, "second prompt"); err != nil {
		t.Fatalf("second prompt never appeared: %v", err)
	}

	windows, err := runSSH(ctx, f.target, "tmux list-windows -t "+testSlug+" -F '#{window_name}'", nil)
	if err != nil {
		t.Fatal(err)
	}
	list := nonEmptyLines(string(windows))
	has := map[string]bool{}
	for _, w := range list {
		has[w] = true
	}
	if !has["cat"] || !has["cat-2"] {
		t.Fatalf("windows = %v, want cat and cat-2 among them", list)
	}
}

func waitForCapture(ctx context.Context, t sshTarget, slug, window, want string) (string, error) {
	deadline := time.Now().Add(5 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		pane, err := capturePane(ctx, t, slug, window)
		if err != nil {
			return pane, err
		}
		last = pane
		if strings.Contains(pane, want) {
			return pane, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return last, exitf(ExitGeneric, "timed out waiting for %q in pane", want)
}
