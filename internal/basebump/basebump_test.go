package basebump

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)

func v(s string, security bool) Version {
	return Version{Version: s, NixRev: "rev-" + s, ReleasedAt: t0, Security: security}
}

// fakeDispatch answers Build and Apply from maps and records the calls.
type fakeDispatch struct {
	mu        sync.Mutex
	buildErr  map[string]error
	kernel    map[string]bool
	applyErr  map[string]error
	builds    []string
	applies   []string
	inflight  map[string]int
	maxPerHst map[string]int
	hostOf    map[string]string
}

func (f *fakeDispatch) Build(_ context.Context, p Project, ver Version) (BuildResult, error) {
	f.mu.Lock()
	f.builds = append(f.builds, p.ID)
	h := f.hostOf[p.ID]
	f.inflight[h]++
	if f.inflight[h] > f.maxPerHst[h] {
		f.maxPerHst[h] = f.inflight[h]
	}
	f.mu.Unlock()
	time.Sleep(2 * time.Millisecond)
	f.mu.Lock()
	f.inflight[h]--
	f.mu.Unlock()
	if err := f.buildErr[p.ID]; err != nil {
		return BuildResult{}, err
	}
	return BuildResult{SystemClosure: "/nix/store/" + p.ID + "-" + ver.Version, KernelChanged: f.kernel[p.ID]}, nil
}

func (f *fakeDispatch) Apply(_ context.Context, p Project, closure string) (ApplyResult, error) {
	f.mu.Lock()
	f.applies = append(f.applies, p.ID)
	f.mu.Unlock()
	if err := f.applyErr[p.ID]; err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{RebootRequired: f.kernel[p.ID]}, nil
}

type fakeRecord struct {
	mu   sync.Mutex
	rows []Outcome
}

func (r *fakeRecord) Record(_ context.Context, o Outcome) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, o)
	return nil
}

func newFake(projects []Project) *fakeDispatch {
	f := &fakeDispatch{buildErr: map[string]error{}, kernel: map[string]bool{}, applyErr: map[string]error{},
		inflight: map[string]int{}, maxPerHst: map[string]int{}, hostOf: map[string]string{}}
	for _, p := range projects {
		f.hostOf[p.ID] = p.HostID
	}
	return f
}

// A runner whose clock jumps straight to each slot.
func instantRunner(d Dispatcher, r Recorder, opts Options) *Runner {
	var mu sync.Mutex
	now := t0
	return &Runner{Dispatch: d, Record: r, Options: opts,
		Now: func() time.Time { mu.Lock(); defer mu.Unlock(); return now },
		Sleep: func(_ context.Context, dur time.Duration) error {
			mu.Lock()
			if t := now.Add(dur); t.After(now) {
				now = t
			}
			mu.Unlock()
			return nil
		}}
}

// The checklist's three projects: one applies, one is held, one fails and
// keeps working on its old base.
func TestThreeProjects(t *testing.T) {
	projects := []Project{
		{ID: "p-apply", HostID: "host-01", State: "running", BaseVersion: "2026.09.3", Fragment: "{}"},
		{ID: "p-held", HostID: "host-01", State: "running", BaseVersion: "2026.09.3", Hold: true},
		{ID: "p-fails", HostID: "host-01", State: "running", BaseVersion: "2026.09.3"},
	}
	next := v("2026.09.4", false)
	plan := NewPlan(next, projects, t0, Options{})
	if len(plan.Slots) != 2 || len(plan.Skipped) != 1 || plan.Skipped[0] != (Skipped{"p-held", SkipHeld}) {
		t.Fatalf("plan %+v", plan)
	}
	d := newFake(projects)
	d.buildErr["p-fails"] = &Error{Code: "eval_failed", Message: "attribute 'nodejs_25' missing at fragment.nix:7:5\n\nerror: ...", FragmentLine: 7}
	rec := &fakeRecord{}
	outs, err := instantRunner(d, rec, Options{}).Run(context.Background(), plan, projects)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Outcome{}
	for _, o := range outs {
		by[o.ProjectID] = o
	}
	if o := by["p-apply"]; o.Status != StatusApplied || o.Event != EventUpdated || o.Closure != "/nix/store/p-apply-2026.09.4" || o.Version != "2026.09.4" {
		t.Fatalf("p-apply: %+v", o)
	}
	if o := by["p-held"]; o.Status != StatusSkipped || o.Reason != SkipHeld {
		t.Fatalf("p-held: %+v", o)
	}
	if o := by["p-fails"]; o.Status != StatusFailed || o.Event != EventUpdateFailed || o.Err.Code != "eval_failed" || o.Err.FragmentLine != 7 || o.Reason != "attribute 'nodejs_25' missing at fragment.nix:7:5" {
		t.Fatalf("p-fails: %+v", o)
	}
	if len(d.applies) != 1 || d.applies[0] != "p-apply" {
		t.Fatalf("applies: %v", d.applies)
	}
	if len(rec.rows) != 3 {
		t.Fatalf("recorded %d outcomes", len(rec.rows))
	}
	// What `repose status` shows for the three afterwards.
	lines := []string{
		StatusLine("2026.09.4", t0, "2026.09.4", false, false),
		StatusLine("2026.09.3", t0, "2026.09.4", true, false),
		StatusLine("2026.09.3", t0, "2026.09.4", false, false),
	}
	want := []string{
		"base 2026.09.4 (applied 2026-09-20)",
		"base 2026.09.3 (2026.09.4 available, held)",
		"base 2026.09.3 (2026.09.4 available)",
	}
	for i := range lines {
		if lines[i] != want[i] {
			t.Fatalf("status %d: %q want %q", i, lines[i], want[i])
		}
	}
	t.Logf("repose status lines:\n  p-apply  %s\n  p-held   %s\n  p-fails  %s (base_update_failed: %s)", lines[0], lines[1], lines[2], by["p-fails"].Reason)

	// The failed project is skipped by the next bump until it applies
	// successfully; the applied one is current.
	projects[0].BaseVersion = "2026.09.4"
	projects[2].LastBuildFailed = true
	again := NewPlan(next, projects, t0, Options{})
	if len(again.Slots) != 0 || len(again.Skipped) != 3 {
		t.Fatalf("second plan %+v", again)
	}
	reasons := map[string]string{}
	for _, s := range again.Skipped {
		reasons[s.ProjectID] = s.Reason
	}
	if reasons["p-apply"] != SkipCurrent || reasons["p-fails"] != SkipLastBuildFailed || reasons["p-held"] != SkipHeld {
		t.Fatalf("reasons %v", reasons)
	}
}

func TestStoppedBuiltOnlyAndKernelChange(t *testing.T) {
	projects := []Project{
		{ID: "p-stopped", HostID: "h", State: "stopped", BaseVersion: "a"},
		{ID: "p-kernel", HostID: "h", State: "running", BaseVersion: "a"},
		{ID: "p-creating", HostID: "h", State: "creating", BaseVersion: "a"},
		{ID: "p-error", HostID: "h", State: "error", BaseVersion: "a"},
	}
	next := v("b", false)
	plan := NewPlan(next, projects, t0, Options{})
	d := newFake(projects)
	d.kernel["p-kernel"] = true
	outs, err := instantRunner(d, nil, Options{}).Run(context.Background(), plan, projects)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Outcome{}
	for _, o := range outs {
		by[o.ProjectID] = o
	}
	if o := by["p-stopped"]; o.Status != StatusBuilt || o.Closure == "" {
		t.Fatalf("stopped: %+v", o)
	}
	if o := by["p-kernel"]; o.Status != StatusNeedsReboot || o.Event != EventUpdateReady || !strings.Contains(o.Reason, "repose config apply --reboot") {
		t.Fatalf("kernel: %+v", o)
	}
	if by["p-creating"].Reason != SkipState || by["p-error"].Reason != SkipState {
		t.Fatalf("states: %+v %+v", by["p-creating"], by["p-error"])
	}
	for _, a := range d.applies {
		if a == "p-stopped" {
			t.Fatal("a stopped guest must not get ApplyConfig")
		}
	}
	if got := StatusLine("a", t0, "b", false, true); got != "base a (b ready; reboot when convenient)" {
		t.Fatalf("status: %q", got)
	}
}

func TestPlanSpreadsOverWindowPerHost(t *testing.T) {
	var projects []Project
	for i := 0; i < 8; i++ {
		projects = append(projects, Project{ID: "p" + string(rune('a'+i)), HostID: "host-01", State: "running", BaseVersion: "a"})
	}
	for i := 0; i < 3; i++ {
		projects = append(projects, Project{ID: "q" + string(rune('a'+i)), HostID: "host-02", State: "running", BaseVersion: "a"})
	}
	plan := NewPlan(v("b", false), projects, t0, Options{})
	starts := map[string][]time.Duration{}
	for _, s := range plan.Slots {
		starts[s.HostID] = append(starts[s.HostID], s.NotBefore.Sub(t0))
	}
	// 8 projects, 2 per host at a time: 4 steps of 6 h across 24 h.
	want1 := []time.Duration{0, 0, 6 * time.Hour, 6 * time.Hour, 12 * time.Hour, 12 * time.Hour, 18 * time.Hour, 18 * time.Hour}
	for i, w := range want1 {
		if starts["host-01"][i] != w {
			t.Fatalf("host-01 starts %v", starts["host-01"])
		}
	}
	// 3 projects: 2 steps of 12 h.
	want2 := []time.Duration{0, 0, 12 * time.Hour}
	for i, w := range want2 {
		if starts["host-02"][i] != w {
			t.Fatalf("host-02 starts %v", starts["host-02"])
		}
	}
	// Security: the same shape inside 2 h.
	sec := NewPlan(v("b", true), projects, t0, Options{})
	last := sec.Slots[len(sec.Slots)-1].NotBefore.Sub(t0)
	if last != 90*time.Minute {
		t.Fatalf("security last start %v", last)
	}
	// The runner never exceeds two builds at once on a host.
	d := newFake(projects)
	if _, err := instantRunner(d, nil, Options{}).Run(context.Background(), plan, projects); err != nil {
		t.Fatal(err)
	}
	if d.maxPerHst["host-01"] > 2 || len(d.builds) != 11 {
		t.Fatalf("max per host %v, builds %d", d.maxPerHst, len(d.builds))
	}
}

func TestApplyFailureAndContextCancel(t *testing.T) {
	projects := []Project{{ID: "p1", HostID: "h", State: "running", BaseVersion: "a"}}
	plan := NewPlan(v("b", false), projects, t0, Options{})
	d := newFake(projects)
	d.applyErr["p1"] = errors.New("guestd Switch: connection reset")
	outs, _ := instantRunner(d, nil, Options{}).Run(context.Background(), plan, projects)
	if outs[0].Status != StatusFailed || outs[0].Err.Code != "internal" || outs[0].Reason != "guestd Switch: connection reset" {
		t.Fatalf("apply failure: %+v", outs[0])
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := instantRunner(newFake(projects), nil, Options{})
	r.Sleep = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	if _, err := r.Run(ctx, plan, projects); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestSummaryAndRollback(t *testing.T) {
	outs := []Outcome{
		{ProjectID: "a", Status: StatusApplied}, {ProjectID: "b", Status: StatusBuilt}, {ProjectID: "c", Status: StatusNeedsReboot},
		{ProjectID: "d", Status: StatusFailed, Reason: "boom"}, {ProjectID: "e", Status: StatusSkipped, Reason: SkipHeld}, {ProjectID: "f", Status: StatusSkipped, Reason: SkipHeld},
	}
	s := Summarize("b", outs)
	if s.Applied != 1 || s.Built != 1 || s.NeedsReboot != 1 || len(s.Failed) != 1 || s.Skipped[SkipHeld] != 2 {
		t.Fatalf("summary %+v", s)
	}
	projects := []Project{
		{ID: "on-new", HostID: "h", State: "running", BaseVersion: "b"},
		{ID: "still-old", HostID: "h", State: "running", BaseVersion: "a"},
		{ID: "held-new", HostID: "h", State: "running", BaseVersion: "b", Hold: true},
	}
	rb := Rollback(v("b", false), v("a", false), projects, t0, Options{})
	if len(rb.Slots) != 1 || rb.Slots[0].ProjectID != "on-new" || rb.Version.Version != "a" {
		t.Fatalf("rollback %+v", rb)
	}
	if got := StatusLine("a", time.Time{}, "", false, false); got != "base a" {
		t.Fatalf("bare status %q", got)
	}
}
