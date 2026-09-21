package basebump_test

import (
	"os"
	"strings"
	"testing"
	"time"

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
	// A newer base gets another try (I-146): the failure was against
	// 2026.09.29, not against this one.
	if _, err := h.Pool.Exec(ctx, "insert into base_versions (version, nix_rev, released_at) values ('2026.09.30', 'fed789', now() + interval '1 second')"); err != nil {
		t.Fatal(err)
	}
	touched, _ = job.Sweep(ctx)
	if len(touched) != 2 {
		t.Fatalf("a failed bump on an older base blocked the next base: touched %v", touched)
	}
	for _, id := range touched {
		ops, _ := store.ListProjectOps(ctx, h.Pool, id, 1)
		if got := h.WaitOp(ops[0].ID); got.State != "done" {
			t.Fatalf("retry on 2026.09.30: %+v", got.Error)
		}
	}
}

// A bump whose build succeeds and whose switch of the running guest fails
// tells the user that, not "failed to build" (I-145).
func TestBumpSwitchFailureSaysSwitch(t *testing.T) {
	h := apitest.New(t, apitest.Options{})
	u := h.NewUser("lou")
	a := h.CreateRunning(u, "a")
	ctx := h.Ctx
	if _, err := h.Pool.Exec(ctx, "insert into base_versions (version, nix_rev, changelog) values ('2026.09.24', 'sw0001', 'guestd')"); err != nil {
		t.Fatal(err)
	}
	job := basebump.New(h.Pool, h.Engine, h.Events, h.Log)
	h.Engine.SetOnFinished(job.OnOpFinished)
	h.Fake.SetFail("ApplyConfig", "guest_unresponsive")
	touched, err := job.Sweep(ctx)
	if err != nil || len(touched) != 1 {
		t.Fatalf("sweep: %v %v", touched, err)
	}
	ops, _ := store.ListProjectOps(ctx, h.Pool, a.ID, 1)
	if got := h.WaitOp(ops[0].ID); got.State != "error" {
		t.Fatalf("op should have failed at the switch: %+v", got)
	}
	revs, _ := store.ListRevisions(ctx, h.Pool, a.ID)
	if revs[0].Status != "built" {
		t.Fatalf("revision after a failed switch: %s", revs[0].Status)
	}
	var summary string
	h.WaitFor("base_update_failed event", func() bool {
		return h.Pool.QueryRow(ctx, "select summary from events where project_id = $1 and kind = 'base_update_failed'", a.ID).Scan(&summary) == nil
	})
	if !strings.Contains(summary, "built, but switching") || strings.Contains(summary, "failed to build") {
		t.Fatalf("summary %q", summary)
	}
}

// A bump whose build changes the kernel ends built with reboot_required
// on a running project; the base_updated event must say so instead of
// "applied" (I-132).
func TestBumpNeedingRebootSaysSo(t *testing.T) {
	h := apitest.New(t, apitest.Options{})
	u := h.NewUser("kay")
	a := h.CreateRunning(u, "a")
	ctx := h.Ctx
	// The project runs on an older published base; the bump must build
	// against the new one, not this.
	if _, err := h.Pool.Exec(ctx, "insert into base_versions (version, nix_rev, changelog, released_at) values ('2026.09.22', 'old01', 'lts', now() - interval '7 days'), ('2026.09.23', 'kern01', 'kernel 7.2.6', now())"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Pool.Exec(ctx, "update projects set base_version = '2026.09.22' where id = $1", a.ID); err != nil {
		t.Fatal(err)
	}
	job := basebump.New(h.Pool, h.Engine, h.Events, h.Log)
	h.Engine.SetOnFinished(job.OnOpFinished)
	h.Fake.SetKernelChanged(true)
	touched, err := job.Sweep(ctx)
	if err != nil || len(touched) != 1 {
		t.Fatalf("sweep: %v %v", touched, err)
	}
	ops, _ := store.ListProjectOps(ctx, h.Pool, a.ID, 1)
	got := h.WaitOp(ops[0].ID)
	// The build ran against the new base's revision, not the one the
	// project was on (I-134: on host-01 every bump had been built from the
	// old checkout, so no base bump ever changed a guest).
	var builds []string
	for _, c := range h.Fake.Commands() {
		if b := c.GetBuild(); b != nil && b.ProjectId == a.ID.String() {
			builds = append(builds, b.BaseRef+"@"+b.BaseVersion)
		}
	}
	if len(builds) == 0 || builds[len(builds)-1] != "kern01@2026.09.23" {
		t.Fatalf("bump build base refs: %v", builds)
	}
	if got.State != "done" || !got.RebootRequired {
		t.Fatalf("bump op: state=%s reboot_required=%v err=%+v", got.State, got.RebootRequired, got.Error)
	}
	revs, _ := store.ListRevisions(ctx, h.Pool, a.ID)
	if revs[0].Status != "built" || !revs[0].RebootRequired || !revs[0].KernelChanged {
		t.Fatalf("revision: status=%s reboot_required=%v kernel_changed=%v", revs[0].Status, revs[0].RebootRequired, revs[0].KernelChanged)
	}
	var summary string
	h.WaitFor("base_updated event", func() bool {
		return h.Pool.QueryRow(ctx, "select summary from events where project_id = $1 and kind = 'base_updated'", a.ID).Scan(&summary) == nil
	})
	if !strings.Contains(summary, "built") || !strings.Contains(summary, "repose stop && repose start") || strings.Contains(summary, "applied") {
		t.Fatalf("summary %q", summary)
	}
}

// A restart inside a security release's ten-minute window must not lose
// its sweep (I-141): what is due is "newer than this process's last
// sweep", and a fresh process has none.
func TestSecurityDueSurvivesRestart(t *testing.T) {
	rel := time.Date(2026, 9, 21, 2, 32, 36, 0, time.UTC)
	sec := &store.BaseVersion{Version: "2026.09.21.3", Security: true, ReleasedAt: rel}
	if !basebump.SecurityDue(sec, time.Time{}) {
		t.Fatal("a fresh process must sweep a security release, however old")
	}
	if basebump.SecurityDue(sec, rel.Add(time.Minute)) {
		t.Fatal("a release already swept is not due again")
	}
	if !basebump.SecurityDue(sec, rel.Add(-time.Minute)) {
		t.Fatal("a release newer than the last sweep is due")
	}
	if basebump.SecurityDue(&store.BaseVersion{Version: "2026.09.22", ReleasedAt: rel}, time.Time{}) {
		t.Fatal("a routine release waits for 04:00")
	}
	if basebump.SecurityDue(nil, time.Time{}) {
		t.Fatal("no base, no sweep")
	}
}
