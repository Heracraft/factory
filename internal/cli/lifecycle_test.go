package cli

import (
	"context"
	"testing"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
)

func newLifecycleEnv(t *testing.T, fake *fakeapi.Fake) *Env {
	t.Helper()
	dir := t.TempDir()
	return &Env{
		Dir: dir, Cfg: defaultConfig(), Cache: newProjectsCache(), Cwd: t.TempDir(),
		Client: newClient(fake.URL()+"/v1", staticToken("tok")),
		Out:    &discardWriter{}, ErrOut: &discardWriter{},
	}
}

func TestStartStopLifecycle(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()
	e := newLifecycleEnv(t, fake)
	ctx := context.Background()

	p, err := e.Client.CreateProject(ctx, CreateProjectRequest{Name: "todo-app", Class: "large"})
	if err != nil {
		t.Fatal(err)
	}

	if err := StartCmd(ctx, e, p.ID); err != nil {
		t.Fatalf("StartCmd: %v", err)
	}
	got, err := e.Client.GetProject(ctx, p.ID)
	if err != nil || got.State != "running" {
		t.Fatalf("state = %q err=%v, want running", got.State, err)
	}

	if err := StopCmd(ctx, e, p.ID, true); err != nil {
		t.Fatalf("StopCmd: %v", err)
	}
	got, err = e.Client.GetProject(ctx, p.ID)
	if err != nil || got.State != "stopped" {
		t.Fatalf("state = %q err=%v, want stopped", got.State, err)
	}
}

func TestDestroyRequiresConfirmation(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()
	e := newLifecycleEnv(t, fake)
	ctx := context.Background()
	p, err := e.Client.CreateProject(ctx, CreateProjectRequest{Name: "todo-app", Class: "large"})
	if err != nil {
		t.Fatal(err)
	}

	err = DestroyCmd(ctx, e, p.ID, false, nil)
	ee, ok := err.(*exitError)
	if !ok || ee.code != ExitUsage {
		t.Fatalf("err = %v, want a usage exitError", err)
	}

	if err := DestroyCmd(ctx, e, p.ID, true, nil); err != nil {
		t.Fatalf("DestroyCmd --yes: %v", err)
	}
	// A destroyed project drops out of GET /projects/:id entirely (the
	// fake's docs: destroyed projects are hidden from the normal lookup,
	// visible only where the api explicitly keeps them for retention).
	if _, err := e.Client.GetProject(ctx, p.ID); err == nil {
		t.Fatal("expected a destroyed project to 404 on GetProject")
	}
}

func TestAttachToStoppedGuestExitsFive(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()
	e := newLifecycleEnv(t, fake)
	ctx := context.Background()
	p, err := e.Client.CreateProject(ctx, CreateProjectRequest{Name: "todo-app", Class: "large"})
	if err != nil {
		t.Fatal(err)
	}
	// The fake starts a freshly created project running at once (its own
	// doc: "operations complete at once"); stop it to get to the state
	// this test wants.
	if _, err := e.Client.StopProject(ctx, p.ID, false); err != nil {
		t.Fatal(err)
	}
	e.Cache.ByDir[e.Cwd] = p.ID

	err = runRun(ctx, e, RunOptions{}, true)
	ee, ok := err.(*exitError)
	if !ok || ee.code != ExitGuestNotRunning {
		t.Fatalf("err = %v, want a guest-not-running exitError", err)
	}
}
