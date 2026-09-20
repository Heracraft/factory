package cli

import (
	"context"
	"testing"
	"time"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
)

// The real api answers a create with state "creating" and an op that runs
// for a while; starting the project meanwhile is a conflict (DECISIONS
// I-106). The run flow must wait on that op, not start.
func TestRunWaitsForTheCreateOpInsteadOfStarting(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{CreateDelay: 400 * time.Millisecond})
	defer fake.Close()
	f := newRunFixture(t, fake)

	if err := runRun(context.Background(), f.env, RunOptions{Name: testSlug, NoAttach: true}, false); err != nil {
		t.Fatalf("runRun: %v", err)
	}
	projects, err := f.env.Client.ListProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].State != "running" {
		t.Fatalf("expected one running project, got %+v", projects)
	}
}
