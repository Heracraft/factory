package basebump_test

import (
	"os"
	"testing"

	"github.com/heracraft/repose/internal/api/apitest"
	"github.com/heracraft/repose/internal/api/basebump"
	"github.com/heracraft/repose/internal/api/ops"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db/testdb"
)

func TestMain(m *testing.M) { os.Exit(testdb.Run(m)) }

// Three projects: one unheld running, one unheld stopped, one held. A
// publish builds the two unheld ones and skips the held one.
func TestSweepBuildsUnheldSkipsHeld(t *testing.T) {
	h := apitest.New(t, apitest.Options{})
	u := h.NewUser("ivy")
	a := h.CreateRunning(u, "a")
	b := h.CreateRunning(u, "b")
	c := h.CreateRunning(u, "c")
	ctx := h.Ctx
	if _, err := h.Pool.Exec(ctx, "update projects set hold_base_updates = true where id = $1", c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Pool.Exec(ctx, "update projects set state = 'stopped' where id = $1", b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Pool.Exec(ctx, "insert into base_versions (version, nix_rev, changelog) values ('2026.09.22', 'abc123', 'claude-code 2.1.280')"); err != nil {
		t.Fatal(err)
	}
	job := basebump.New(h.Pool, h.Engine, h.Events, h.Log)
	h.Engine.SetOnFinished(job.OnOpFinished)
	touched, err := job.Sweep(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(touched) != 2 {
		t.Fatalf("touched %v", touched)
	}
	for _, id := range touched {
		if id == c.ID {
			t.Fatal("held project was bumped")
		}
	}
	for _, p := range []*store.Project{a, b} {
		ops, _ := store.ListProjectOps(ctx, h.Pool, p.ID, 5)
		if len(ops) == 0 || ops[0].Kind != "build" {
			t.Fatalf("no build op for %s", p.Slug)
		}
		got := h.WaitOp(ops[0].ID)
		if got.State != "done" {
			t.Fatalf("bump build for %s: %+v", p.Slug, got.Error)
		}
	}
	h.WaitFor("running project on the new base", func() bool {
		pa := h.Project(a.ID)
		return pa.BaseVersion != nil && *pa.BaseVersion == "2026.09.22"
	})
	// The stopped project's revision stays built, and its base moves, at
	// its next start.
	pb := h.Project(b.ID)
	revs, _ := store.ListRevisions(ctx, h.Pool, pb.ID)
	if revs[0].Status != "built" || pb.BaseVersion == nil || *pb.BaseVersion == "2026.09.22" {
		t.Fatalf("stopped project's bump: revision %s base %v", revs[0].Status, pb.BaseVersion)
	}
	if h.Project(c.ID).BaseVersion != nil && *h.Project(c.ID).BaseVersion == "2026.09.22" {
		t.Fatal("held project moved to the new base")
	}
	// A second sweep is a no-op for the updated projects.
	again, _ := job.Sweep(ctx)
	if len(again) != 0 {
		t.Fatalf("second sweep touched %v", again)
	}
	var n int
	h.WaitFor("base_updated events", func() bool {
		_ = h.Pool.QueryRow(ctx, "select count(*) from events where kind = 'base_updated'").Scan(&n)
		return n == 2
	})
	// The stopped project starts and applies the pending base revision.
	bid := b.ID
	sop := h.WaitOp(h.Enqueue(ops.NewOp{Kind: ops.KindStart, ProjectID: &bid, Phases: ops.PlanStart(true)}))
	if sop.State != "done" {
		t.Fatalf("start with pending revision: %+v", sop.Error)
	}
	if pb := h.Project(b.ID); pb.State != "running" || pb.BaseVersion == nil || *pb.BaseVersion != "2026.09.22" {
		t.Fatalf("after start: state=%s base=%v", pb.State, pb.BaseVersion)
	}
	// A failing bump raises base_update_failed and keeps the project working.
	if _, err := h.Pool.Exec(ctx, "insert into base_versions (version, nix_rev) values ('2026.09.29', 'def456')"); err != nil {
		t.Fatal(err)
	}
	h.Fake.SetFail("Build", "build_failed")
	touched, _ = job.Sweep(ctx)
	for _, id := range touched {
		ops, _ := store.ListProjectOps(ctx, h.Pool, id, 1)
		h.WaitOp(ops[0].ID)
	}
	h.WaitFor("base_update_failed events", func() bool {
		_ = h.Pool.QueryRow(ctx, "select count(*) from events where kind = 'base_update_failed'").Scan(&n)
		return n == 2
	})
	if h.Project(a.ID).State != "running" || *h.Project(a.ID).BaseVersion != "2026.09.22" {
		t.Fatal("a failed bump changed the project")
	}
	h.Fake.SetFail("Build", "")
	if touched, _ := job.Sweep(ctx); len(touched) != 0 {
		t.Fatalf("projects with a failed bump were retried: %v", touched)
	}
}
