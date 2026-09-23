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

	copied, err := syncCredentials(context.Background(), f.target, home, f.local, credSyncOptions{RemoteURL: "github.com/a/b", ghToken: func() string { t.Fatal("hosts.yml has a token; the keyring must not be asked"); return "" }})
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
	// gh travelled and the remote is on github: the guest's git reaches
	// github over HTTPS with gh as the helper, so an agent can push
	// without the laptop's SSH keys (I-150).
	if got := mustRun(t, f.guestHome, "git", "config", "--file", filepath.Join(f.guestHome, ".gitconfig"), "url.https://github.com/.insteadOf"); got != "git@github.com:" {
		t.Fatalf("insteadOf = %q", got)
	}
	if got := mustRun(t, f.guestHome, "git", "config", "--file", filepath.Join(f.guestHome, ".gitconfig"), "credential.https://github.com.helper"); got != "!gh auth git-credential" {
		t.Fatalf("credential helper = %q", got)
	}
}

// gh 2.40+ keeps the token in the laptop's keyring and hosts.yml has none;
// the guest has no keyring, so the token is written into the hosts.yml
// that travels.
func TestSyncCredentialsCarriesAKeyringGhToken(t *testing.T) {
	f := newSyncFixture(t)
	home := t.TempDir()
	p := filepath.Join(home, ".config", "gh", "hosts.yml")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("github.com:\n    git_protocol: ssh\n    users:\n        dev:\n    user: dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	copied, err := syncCredentials(context.Background(), f.target, home, f.local, credSyncOptions{RemoteURL: "gitlab.com/a/b", ghToken: func() string { return "gho_fromkeyring" }})
	if err != nil {
		t.Fatalf("syncCredentials: %v", err)
	}
	if len(copied) == 0 || copied[0] != "gh" {
		t.Fatalf("copied = %v", copied)
	}
	b, err := os.ReadFile(filepath.Join(f.guestHome, ".config", "gh", "hosts.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "github.com:\n    oauth_token: gho_fromkeyring\n    git_protocol: ssh") {
		t.Fatalf("token not written under github.com:\n%s", b)
	}
	if local, _ := os.ReadFile(p); strings.Contains(string(local), "oauth_token") {
		t.Fatal("the laptop's hosts.yml was changed")
	}
	if cfg, err := os.ReadFile(filepath.Join(f.guestHome, ".gitconfig")); err == nil && strings.Contains(string(cfg), "insteadOf") {
		t.Fatalf("insteadOf set for a non-github remote:\n%s", cfg)
	}
}

// features/secrets.md: a login done inside the guest (newer than the
// laptop's file) is not clobbered, and the CLI says which side won.
func TestSyncCredentialsKeepsANewerGuestLogin(t *testing.T) {
	f := newSyncFixture(t)
	home := t.TempDir()
	local := filepath.Join(home, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(local), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte(`{"from":"laptop"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(local, old, old); err != nil {
		t.Fatal(err)
	}
	guestFile := filepath.Join(f.guestHome, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(guestFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guestFile, []byte(`{"from":"guest"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var kept []string
	copied, err := syncCredentials(context.Background(), f.target, home, f.local, credSyncOptions{Kept: func(l string) { kept = append(kept, l) }})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(guestFile); string(b) != `{"from":"guest"}` {
		t.Fatalf("the guest's newer login was overwritten: %s", b)
	}
	if len(kept) != 1 || kept[0] != "codex" {
		t.Fatalf("kept = %v", kept)
	}
	for _, c := range copied {
		if c == "codex" {
			t.Fatalf("codex reported as copied: %v", copied)
		}
	}

	// Once the laptop's is newer, it wins.
	if err := os.Chtimes(local, time.Now().Add(time.Hour), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := syncCredentials(context.Background(), f.target, home, f.local, credSyncOptions{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(guestFile); string(b) != `{"from":"laptop"}` {
		t.Fatalf("the laptop's newer login did not arrive: %s", b)
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
