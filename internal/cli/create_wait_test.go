package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
)

// TestRunWaitsForANameHeldByADestroy is I-407: `repose rm recruiting` and
// then `repose run` in a checkout that recruiting was not found by (it had
// no remote) met "a project named recruiting already exists" while the
// destroy ran, and made recruiting-2 (dogfood 2026-10-01). run now waits
// for the destroy and creates the name it asked for.
func TestRunWaitsForANameHeldByADestroy(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{DestroyDelay: 1500 * time.Millisecond})
	defer fake.Close()
	e := newLifecycleEnv(t, fake)
	errOut := &discardWriter{}
	e.ErrOut = errOut
	ctx := context.Background()
	old, err := e.Client.CreateProject(ctx, CreateProjectRequest{Name: "recruiting", Class: "large"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Client.DestroyProject(ctx, old.ID); err != nil {
		t.Fatal(err)
	}
	pr := newProgress(errOut, false)
	p, err := createProjectForRun(ctx, e, "", RunOptions{Name: "recruiting"}, pr)
	pr.End()
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.Slug != "recruiting" || p.ID == old.ID {
		t.Fatalf("created %s (%s), want a fresh recruiting", p.Slug, p.ID)
	}
	out := errOut.buf.String()
	if strings.Contains(out, "is taken") || !strings.Contains(out, "Waiting for the old recruiting to finish destroying") {
		t.Fatalf("printed %q", out)
	}

	// A live project holding the name: NAME-2, as before.
	q, err := createProjectForRun(ctx, e, "", RunOptions{Name: "recruiting"}, nil)
	if err != nil || q.Slug != "recruiting-2" {
		t.Fatalf("second create: %v %+v", err, q)
	}
}
