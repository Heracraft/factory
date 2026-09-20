// Package basebump rolls a new platform base version out to projects
// (docs/workstreams/12-nix-config-pipeline.md "Base bumps", DECISIONS
// R4-5). The api owns the tables and the gRPC stream; this package owns
// the policy: which projects a published version reaches, when, how many
// at once per host, and what each outcome means. It is a planner and a
// runner over two small interfaces so it is tested without Postgres or a
// host, and so hostdev can drive the same policy for one host.
package basebump

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Version is a base_versions row (docs/interfaces/db-schema.md).
type Version struct {
	Version    string
	NixRev     string
	Changelog  string
	ReleasedAt time.Time
	Security   bool
}

// Project is what the planner needs to know about one project.
type Project struct {
	ID          string
	HostID      string
	State       string // the guest state enum; only running and stopped are bumped
	BaseVersion string // the base the project's applied revision was built on
	Hold        bool   // projects.hold_base_updates
	// LastBuildFailed is set when the project's newest config revision is
	// failed; a bump never touches such a project until the user's next
	// successful apply, because the failure is theirs to see first.
	LastBuildFailed bool
	Fragment        string // the project's current fragment, rebuilt on the new base
}

// Skip reasons.
const (
	SkipHeld            = "held"
	SkipLastBuildFailed = "last_build_failed"
	SkipState           = "state"
	SkipCurrent         = "already_on_version"
)

// Outcome statuses.
const (
	StatusApplied     = "applied"      // built and switched in place
	StatusBuilt       = "built"        // stopped guest: rooted, used at next start
	StatusNeedsReboot = "needs_reboot" // built; the kernel changed and the user decides
	StatusFailed      = "failed"       // build or apply failed; the project keeps its base
	StatusSkipped     = "skipped"
)

// Event kinds a bump raises on a project (workstream 13 delivers them).
const (
	EventUpdateFailed = "base_update_failed"
	EventUpdateReady  = "base_update_ready"
	EventUpdated      = "base_updated"
)

// Slot is one project's place in the rollout.
type Slot struct {
	ProjectID string
	HostID    string
	NotBefore time.Time
}

// Skipped is a project the plan leaves alone and why.
type Skipped struct {
	ProjectID string
	Reason    string
}

// Plan is a rollout: which projects, in what order, not before when.
type Plan struct {
	Version Version
	Slots   []Slot
	Skipped []Skipped
}

// Options tune a plan.
type Options struct {
	// Window is how long the rollout is spread over: 24 h, or 2 h for a
	// security release (the doc's numbers are the defaults).
	Window         time.Duration
	SecurityWindow time.Duration
	// PerHost is how many builds run at once on one host (the host's own
	// max-jobs; 2).
	PerHost int
}

func (o Options) withDefaults() Options {
	if o.Window == 0 {
		o.Window = 24 * time.Hour
	}
	if o.SecurityWindow == 0 {
		o.SecurityWindow = 2 * time.Hour
	}
	if o.PerHost == 0 {
		o.PerHost = 2
	}
	return o
}

// NewPlan decides which projects a version reaches and spreads them over
// the window, per host, so no host sees more than PerHost concurrent
// builds and the fleet is not rebuilt in the same minute. Eligible
// projects are ordered by id (UUIDv7: oldest first) and given start times
// at even intervals over the window on each host.
func NewPlan(v Version, projects []Project, now time.Time, opts Options) Plan {
	opts = opts.withDefaults()
	window := opts.Window
	if v.Security {
		window = opts.SecurityWindow
	}
	p := Plan{Version: v}
	byHost := map[string][]Project{}
	for _, pr := range projects {
		switch {
		case pr.Hold:
			p.Skipped = append(p.Skipped, Skipped{pr.ID, SkipHeld})
		case pr.LastBuildFailed:
			p.Skipped = append(p.Skipped, Skipped{pr.ID, SkipLastBuildFailed})
		case pr.State != "running" && pr.State != "stopped":
			p.Skipped = append(p.Skipped, Skipped{pr.ID, SkipState})
		case pr.BaseVersion == v.Version:
			p.Skipped = append(p.Skipped, Skipped{pr.ID, SkipCurrent})
		default:
			byHost[pr.HostID] = append(byHost[pr.HostID], pr)
		}
	}
	hosts := make([]string, 0, len(byHost))
	for h := range byHost {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	for _, h := range hosts {
		prs := byHost[h]
		sort.Slice(prs, func(i, j int) bool { return prs[i].ID < prs[j].ID })
		// PerHost projects may start together; the rest follow at even
		// steps so the last one starts before the window closes.
		steps := (len(prs) + opts.PerHost - 1) / opts.PerHost
		var step time.Duration
		if steps > 1 {
			step = window / time.Duration(steps)
		}
		for i, pr := range prs {
			p.Slots = append(p.Slots, Slot{ProjectID: pr.ID, HostID: h, NotBefore: now.Add(step * time.Duration(i/opts.PerHost))})
		}
	}
	sort.SliceStable(p.Slots, func(i, j int) bool {
		if !p.Slots[i].NotBefore.Equal(p.Slots[j].NotBefore) {
			return p.Slots[i].NotBefore.Before(p.Slots[j].NotBefore)
		}
		return p.Slots[i].ProjectID < p.Slots[j].ProjectID
	})
	sort.Slice(p.Skipped, func(i, j int) bool { return p.Skipped[i].ProjectID < p.Skipped[j].ProjectID })
	return p
}

// BuildResult is what the dispatcher's Build returns (grpc-hostd.md).
type BuildResult struct {
	SystemClosure string
	ClosureBytes  uint64
	KernelChanged bool
}

// ApplyResult is what ApplyConfig returns.
type ApplyResult struct {
	Rebooted       bool
	RebootRequired bool
}

// Error is a build or apply failure with the interface's code.
type Error struct {
	Code         string
	Message      string
	FragmentLine int32
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Dispatcher sends Build and ApplyConfig for a project; the api's
// implementation writes the config_revisions row and talks to the host.
type Dispatcher interface {
	// Build rebuilds the project's fragment on the version's nix_rev.
	Build(ctx context.Context, project Project, v Version) (BuildResult, error)
	// Apply switches a running guest to the closure without forcing a
	// reboot.
	Apply(ctx context.Context, project Project, closure string) (ApplyResult, error)
}

// Outcome is the recorded result for one project.
type Outcome struct {
	ProjectID string
	Version   string
	Status    string
	Reason    string // skip reason, or the error's first line
	Closure   string
	Err       *Error
	Event     string // event kind raised, "" for none
	At        time.Time
}

// Recorder persists outcomes: the api updates the project's base version
// on applied/built, inserts the event, and marks the revision failed.
type Recorder interface {
	Record(ctx context.Context, o Outcome) error
}

// Runner executes a plan.
type Runner struct {
	Dispatch Dispatcher
	Record   Recorder
	Options  Options
	// Now and Sleep are injectable for tests; nil means the wall clock.
	Now   func() time.Time
	Sleep func(ctx context.Context, d time.Duration) error
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Runner) sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	if r.Sleep != nil {
		return r.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run executes every slot of the plan: waits for its time, holds a
// per-host semaphore of PerHost, builds, applies when the guest runs,
// records the outcome. Skipped projects are recorded once up front. Run
// returns the outcomes in the order they finished and the first context
// error, if any; a failed project never stops the sweep.
func (r *Runner) Run(ctx context.Context, plan Plan, projects []Project) ([]Outcome, error) {
	opts := r.Options.withDefaults()
	byID := map[string]Project{}
	for _, p := range projects {
		byID[p.ID] = p
	}
	var mu sync.Mutex
	var outcomes []Outcome
	record := func(o Outcome) {
		o.Version = plan.Version.Version
		o.At = r.now()
		if r.Record != nil {
			_ = r.Record.Record(ctx, o) // a recorder failure is logged by the api; the sweep goes on
		}
		mu.Lock()
		outcomes = append(outcomes, o)
		mu.Unlock()
	}
	for _, s := range plan.Skipped {
		record(Outcome{ProjectID: s.ProjectID, Status: StatusSkipped, Reason: s.Reason})
	}
	sems := map[string]chan struct{}{}
	for _, s := range plan.Slots {
		if _, ok := sems[s.HostID]; !ok {
			sems[s.HostID] = make(chan struct{}, opts.PerHost)
		}
	}
	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once
	for _, s := range plan.Slots {
		pr, ok := byID[s.ProjectID]
		if !ok {
			record(Outcome{ProjectID: s.ProjectID, Status: StatusSkipped, Reason: "unknown project"})
			continue
		}
		wg.Add(1)
		go func(s Slot, pr Project) {
			defer wg.Done()
			if err := ctx.Err(); err != nil {
				errOnce.Do(func() { firstErr = err })
				return
			}
			if err := r.sleep(ctx, s.NotBefore.Sub(r.now())); err != nil {
				errOnce.Do(func() { firstErr = err })
				return
			}
			sem := sems[s.HostID]
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errOnce.Do(func() { firstErr = ctx.Err() })
				return
			}
			defer func() { <-sem }()
			record(r.bump(ctx, pr, plan.Version))
		}(s, pr)
	}
	wg.Wait()
	sort.SliceStable(outcomes, func(i, j int) bool { return outcomes[i].At.Before(outcomes[j].At) })
	return outcomes, firstErr
}

// bump is one project's rollout: build, then apply when it runs.
func (r *Runner) bump(ctx context.Context, pr Project, v Version) Outcome {
	o := Outcome{ProjectID: pr.ID}
	res, err := r.Dispatch.Build(ctx, pr, v)
	if err != nil {
		return failed(o, err)
	}
	o.Closure = res.SystemClosure
	if pr.State != "running" {
		o.Status, o.Event = StatusBuilt, EventUpdated
		return o
	}
	ar, err := r.Dispatch.Apply(ctx, pr, res.SystemClosure)
	if err != nil {
		return failed(o, err)
	}
	if ar.RebootRequired {
		o.Status, o.Event = StatusNeedsReboot, EventUpdateReady
		o.Reason = "base update ready; reboot when convenient: repose config apply --reboot"
		return o
	}
	o.Status, o.Event = StatusApplied, EventUpdated
	return o
}

func failed(o Outcome, err error) Outcome {
	o.Status, o.Event = StatusFailed, EventUpdateFailed
	var be *Error
	if errors.As(err, &be) {
		o.Err = be
	} else {
		o.Err = &Error{Code: "internal", Message: err.Error()}
	}
	o.Reason = firstLine(o.Err.Message)
	return o
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}

// Summary is the `repose-admin base status <version>` view.
type Summary struct {
	Version     string
	Applied     int
	Built       int
	NeedsReboot int
	Failed      []Outcome
	Skipped     map[string]int // by reason
}

// Summarize counts a rollout's outcomes.
func Summarize(version string, outcomes []Outcome) Summary {
	s := Summary{Version: version, Skipped: map[string]int{}}
	for _, o := range outcomes {
		switch o.Status {
		case StatusApplied:
			s.Applied++
		case StatusBuilt:
			s.Built++
		case StatusNeedsReboot:
			s.NeedsReboot++
		case StatusFailed:
			s.Failed = append(s.Failed, o)
		case StatusSkipped:
			s.Skipped[o.Reason]++
		}
	}
	return s
}

// StatusLine is the base part of `repose status` for one project
// (docs/workstreams/12-nix-config-pipeline.md): "base 2026.09.3 (2026.09.4
// available, held)", "base 2026.09.3 (2026.09.4 available)", "base
// 2026.09.4 (applied 2026-09-20)", or "base 2026.09.3 (2026.09.4 ready;
// reboot when convenient)".
func StatusLine(current string, appliedAt time.Time, latest string, held bool, rebootPending bool) string {
	switch {
	case latest != "" && latest != current && rebootPending:
		return fmt.Sprintf("base %s (%s ready; reboot when convenient)", current, latest)
	case latest != "" && latest != current && held:
		return fmt.Sprintf("base %s (%s available, held)", current, latest)
	case latest != "" && latest != current:
		return fmt.Sprintf("base %s (%s available)", current, latest)
	case !appliedAt.IsZero():
		return fmt.Sprintf("base %s (applied %s)", current, appliedAt.UTC().Format("2006-01-02"))
	}
	return "base " + current
}

// Rollback is the plan that puts every project now on `from` back on `to`
// (`repose-admin base rollback`): the same policy, previous version, and
// only projects the bump reached. The closures are still rooted for the
// newest three revisions, so the builds are cache hits.
func Rollback(from, to Version, projects []Project, now time.Time, opts Options) Plan {
	var reached []Project
	for _, p := range projects {
		if p.BaseVersion == from.Version {
			reached = append(reached, p)
		}
	}
	return NewPlan(to, reached, now, opts)
}
