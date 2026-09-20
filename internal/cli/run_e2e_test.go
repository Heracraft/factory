package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
)

// newRunFixture wires an Env against a fresh fakeapi.Fake and a
// testguest.Guest whose $HOME/<slug> is a clone of a bare "origin"
// repository — the same shape as newSyncFixture, but exercised through
// runRun end to end rather than through syncGuest directly.
type runFixture struct {
	*syncFixture
	env *Env
}

func newRunFixture(t *testing.T, fake *fakeapi.Fake) *runFixture {
	t.Helper()
	sf := newSyncFixture(t)
	// A real guest's tmux session is created at boot by
	// repose-tmux-session.service (docs/interfaces/guest-conventions.md
	// "tmux"), independent of anything the CLI does; the fixture must
	// have it before runRun looks for windows in it.
	if _, err := runSSH(context.Background(), sf.target, "tmux new-session -d -s "+testSlug+" -c ~/"+testSlug, nil); err != nil {
		t.Fatalf("seeding the guest tmux session: %v", err)
	}
	dir := t.TempDir()
	home := withHome(t) // repoints $HOME so ensureIdentityKey and ensureCert write under a scratch dir
	cache := newProjectsCache()
	env := &Env{
		Dir: dir, Cfg: defaultConfig(), Cache: cache, Cwd: sf.local, HomeDir: home,
		Client: newClient(fake.URL()+"/v1", staticToken("tok")),
		Out:    &discardWriter{}, ErrOut: &discardWriter{},
		TargetFor: func(slug string) sshTarget { return sf.target },
	}
	return &runFixture{syncFixture: sf, env: env}
}

type discardWriter struct{ buf strings.Builder }

func (w *discardWriter) Write(p []byte) (int, error) { return w.buf.Write(p) }

func TestRunCreatesStartsAndAttachesNewProject(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()
	f := newRunFixture(t, fake)

	err := runRun(context.Background(), f.env, RunOptions{Name: testSlug, NoAttach: true}, false)
	if err != nil {
		t.Fatalf("runRun: %v", err)
	}

	projects, err := f.env.Client.ListProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Slug != testSlug || projects[0].State != "running" {
		t.Fatalf("projects = %+v", projects)
	}
	if f.env.Cache.ByDir[f.local] != projects[0].ID {
		t.Fatalf("by_dir cache not populated: %+v", f.env.Cache.ByDir)
	}
}

func TestRunSecondTimeAttachesExistingProject(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()
	f := newRunFixture(t, fake)
	ctx := context.Background()

	if err := runRun(ctx, f.env, RunOptions{Name: testSlug, NoAttach: true}, false); err != nil {
		t.Fatalf("first runRun: %v", err)
	}
	// A fresh Env (as a new process invocation would have), same cache
	// file and home, must resolve the existing project via by_dir and not
	// try to create a second one.
	reloadedCache, err := loadProjectsCache(f.env.Dir)
	if err != nil {
		t.Fatal(err)
	}
	env2 := &Env{
		Dir: f.env.Dir, Cfg: f.env.Cfg, Cache: reloadedCache, Cwd: f.local, HomeDir: f.env.HomeDir,
		Client: f.env.Client, Out: &discardWriter{}, ErrOut: &discardWriter{}, TargetFor: f.env.TargetFor,
	}
	if err := runRun(ctx, env2, RunOptions{NoAttach: true}, false); err != nil {
		t.Fatalf("second runRun: %v", err)
	}
	projects, err := f.env.Client.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected exactly one project, got %d: %+v", len(projects), projects)
	}
}

func TestRunWithPromptSendsIntoTmuxWindow(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()
	f := newRunFixture(t, fake)
	ctx := context.Background()

	if err := runRun(ctx, f.env, RunOptions{
		Name: testSlug, Agent: "cat", Prompt: "finish the feature", NoAttach: true,
	}, false); err != nil {
		t.Fatalf("runRun: %v", err)
	}

	pane, err := waitForCapture(ctx, f.target, testSlug, "cat", "finish the feature")
	if err != nil {
		t.Fatalf("prompt never reached the agent window: %v\n%s", err, pane)
	}
}

// TestRunClaudeNotLoggedInAttachesInstead is 07-cli.md §5.5 step 7: no
// ~/.claude/.credentials.json in the guest and no CLAUDE_CODE_OAUTH_TOKEN
// secret means attach instead of sending, so the user can finish the
// login themselves.
func TestRunClaudeNotLoggedInAttachesInstead(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()
	f := newRunFixture(t, fake)
	ctx := context.Background()

	var out strings.Builder
	f.env.Out = &out

	if err := runRun(ctx, f.env, RunOptions{
		Name: testSlug, Agent: "claude", Prompt: "finish the feature", NoAttach: true,
	}, false); err != nil {
		t.Fatalf("runRun: %v", err)
	}
	if !strings.Contains(out.String(), "Claude Code is not logged in") {
		t.Fatalf("expected the not-logged-in message, got: %s", out.String())
	}

	exists, err := windowExists(ctx, f.target, testSlug, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected a claude window to be opened even though the prompt was not sent")
	}
	pane, err := capturePane(ctx, f.target, testSlug, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(pane, "finish the feature") {
		t.Fatalf("prompt must not have been sent: %s", pane)
	}
}

func TestRunDirtyRemoteTreeRefusesWithExitSix(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()
	f := newRunFixture(t, fake)
	ctx := context.Background()

	if err := runRun(ctx, f.env, RunOptions{Name: testSlug, NoAttach: true}, false); err != nil {
		t.Fatalf("first runRun: %v", err)
	}
	if err := os.WriteFile(filepath.Join(f.guestRepo(), "README.md"), []byte("agent left this dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	env2 := &Env{
		Dir: f.env.Dir, Cfg: f.env.Cfg, Cache: f.env.Cache, Cwd: f.local, HomeDir: f.env.HomeDir,
		Client: f.env.Client, Out: &discardWriter{}, ErrOut: &discardWriter{}, TargetFor: f.env.TargetFor,
	}
	err := runRun(ctx, env2, RunOptions{NoAttach: true}, false)
	ee, ok := err.(*exitError)
	if !ok || ee.code != ExitDirtyRemoteTree {
		t.Fatalf("err = %v, want an exitError with code %d", err, ExitDirtyRemoteTree)
	}
}
