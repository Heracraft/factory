package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
)

// countingTransport counts the op reads (plain and held) and project
// reads a client sends, and can answer the first few op reads with a
// proxy's 502 as a Coolify rolling redeploy does.
type countingTransport struct {
	next      http.RoundTripper
	opReads   atomic.Int32
	opWaits   atomic.Int32
	projReads atomic.Int32
	fail502   atomic.Int32 // op reads still to answer with a 502
}

var opPath = regexp.MustCompile(`/ops/[^/]+$`)
var projPath = regexp.MustCompile(`/projects/[^/]+$`)

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	switch {
	case r.Method == http.MethodGet && opPath.MatchString(r.URL.Path):
		c.opReads.Add(1)
		if r.URL.Query().Get("wait") != "" {
			c.opWaits.Add(1)
		}
		if c.fail502.Load() > 0 {
			c.fail502.Add(-1)
			return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("Bad Gateway")), Header: http.Header{}, Request: r}, nil
		}
	case r.Method == http.MethodGet && projPath.MatchString(r.URL.Path):
		c.projReads.Add(1)
	}
	return c.next.RoundTrip(r)
}

// stoppedProject is a fake with StartDelay and one stopped project, and a
// client counting its reads.
func stoppedProject(t *testing.T, opts fakeapi.Options) (*fakeapi.Fake, *Client, *countingTransport, *Project) {
	t.Helper()
	fake := fakeapi.New(opts)
	t.Cleanup(fake.Close)
	fp, err := fake.CreateProject("wait", "small")
	if err != nil {
		t.Fatal(err)
	}
	fake.SetState(fp.ID, "stopped")
	ct := &countingTransport{next: http.DefaultTransport}
	c := newClient(fake.URL()+"/v1", staticToken("tok"))
	c.HTTP = &http.Client{Transport: ct, Timeout: 30 * time.Second}
	p, err := c.GetProject(context.Background(), fp.ID)
	if err != nil {
		t.Fatal(err)
	}
	ct.projReads.Store(0)
	return fake, c, ct, p
}

func waitEnv(c *Client) (*Env, *bytes.Buffer) {
	var errOut bytes.Buffer
	return &Env{Client: c, Out: io.Discard, ErrOut: &errOut}, &errOut
}

// I-236: against an api that long-polls, a start is waited on with a
// handful of held reads (no project reads), the phase label still moves
// to "Booting", and the wait ends within a few tens of ms of the op.
func TestWaitOpLongPoll(t *testing.T) {
	const delay = 1500 * time.Millisecond
	_, c, ct, p := stoppedProject(t, fakeapi.Options{StartDelay: delay})
	e, errOut := waitEnv(c)
	pr := newProgress(errOut, false)
	ctx := context.Background()
	sr, err := c.StartProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	began := time.Now()
	op, err := waitOpPhased(ctx, e, p, sr.OpID, pr, true)
	el := time.Since(began)
	if err != nil || op.State != "done" {
		t.Fatalf("wait: %v %+v", err, op)
	}
	if el > delay+300*time.Millisecond {
		t.Fatalf("wait ended %s after the start; the op took %s", el, delay)
	}
	// One plain read (it learns the api long-polls), then held reads: the
	// one the phase change ends, and the one the op's end does.
	if n := ct.opReads.Load(); n > 4 {
		t.Fatalf("%d op reads for a %s start; want at most 4", n, delay)
	}
	if ct.opWaits.Load() == 0 {
		t.Fatal("no held read")
	}
	if n := ct.projReads.Load(); n > 1 {
		t.Fatalf("%d project reads; the op read carries the state", n)
	}
	if !strings.Contains(errOut.String(), "Booting wait...") {
		t.Fatalf("no Booting phase: %q", errOut.String())
	}
}

// An older api (no version, ?wait ignored): the wait polls as before, with
// the project read beside each op read, and still shows the phase.
func TestWaitOpOldAPIFallsBackToPolling(t *testing.T) {
	const delay = 1200 * time.Millisecond
	_, c, ct, p := stoppedProject(t, fakeapi.Options{StartDelay: delay, NoLongPoll: true})
	e, errOut := waitEnv(c)
	pr := newProgress(errOut, false)
	ctx := context.Background()
	sr, err := c.StartProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	op, err := waitOpPhased(ctx, e, p, sr.OpID, pr, true)
	if err != nil || op.State != "done" {
		t.Fatalf("wait: %v %+v", err, op)
	}
	if ct.opWaits.Load() != 0 {
		t.Fatalf("%d held reads sent to an api without them", ct.opWaits.Load())
	}
	// 1.2 s at 500 ms between reads: 3 or 4 op reads, as many project reads.
	if n := ct.opReads.Load(); n < 2 || n > 5 {
		t.Fatalf("%d op reads", n)
	}
	if ct.projReads.Load() != ct.opReads.Load() {
		t.Fatalf("project reads %d, op reads %d: want one beside each", ct.projReads.Load(), ct.opReads.Load())
	}
	if !strings.Contains(errOut.String(), "Booting wait...") {
		t.Fatalf("no Booting phase: %q", errOut.String())
	}
}

// The api's waiter bound reached (an answer to ?wait without the header):
// the next read is not sent at once but after the usual pause.
func TestWaitOpUnheldReadPauses(t *testing.T) {
	var mu sync.Mutex
	var times []time.Time
	srv := http.NewServeMux()
	srv.HandleFunc("GET /v1/projects/p/ops/o", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		n := len(times)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		state := "running"
		if n >= 4 {
			state = "done"
		}
		_, _ = io.WriteString(w, `{"state":"`+state+`","version":"v1","project_state":"starting"}`)
	})
	fake := newTestServer(t, srv)
	c := newClient(fake+"/v1", staticToken("tok"))
	op, err := waitOp(context.Background(), c, "p", "o", io.Discard)
	if err != nil || op.State != "done" {
		t.Fatalf("wait: %v %+v", err, op)
	}
	// The first read learns the version and asks again at once; after
	// that every answer came back unheld, so each read waits its pause.
	for i := 2; i < len(times); i++ {
		if gap := times[i].Sub(times[i-1]); gap < opPollInterval-50*time.Millisecond {
			t.Fatalf("read %d came %s after the one before; want the %s pause", i+1, gap, opPollInterval)
		}
	}
}

// A Coolify rolling redeploy: the proxy answers 502 for a moment. The
// wait rides it out instead of failing the command.
func TestWaitOpRidesOutA502(t *testing.T) {
	_, c, ct, p := stoppedProject(t, fakeapi.Options{StartDelay: 800 * time.Millisecond})
	prev := opTransientPause
	opTransientPause = 50 * time.Millisecond
	defer func() { opTransientPause = prev }()
	ctx := context.Background()
	sr, err := c.StartProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	ct.fail502.Store(3)
	op, err := waitOp(ctx, c, p.ID, sr.OpID, io.Discard)
	if err != nil || op.State != "done" {
		t.Fatalf("wait through 502s: %v %+v", err, op)
	}
	// A 502 for longer than the budget is still an error.
	prevB := opTransientBudget
	opTransientBudget = 200 * time.Millisecond
	defer func() { opTransientBudget = prevB }()
	ct.fail502.Store(1000)
	if _, err := waitOp(ctx, c, p.ID, sr.OpID, io.Discard); err == nil {
		t.Fatal("a proxy that never recovers did not fail the wait")
	}
}

// A 4xx is not a proxy blip: it fails the wait at once.
func TestWaitOpNotFoundFailsAtOnce(t *testing.T) {
	_, c, _, p := stoppedProject(t, fakeapi.Options{})
	began := time.Now()
	_, err := waitOp(context.Background(), c, p.ID, "01900000-0000-7000-8000-000000009999", io.Discard)
	var ae *APIError
	if err == nil || !asAPIError(err, &ae) || ae.Code != "not_found" || time.Since(began) > time.Second {
		t.Fatalf("missing op: %v after %s", err, time.Since(began))
	}
}

func newTestServer(t *testing.T, h http.Handler) string {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s.URL
}

// I-237: a run that starts a stopped guest makes its first connection the
// moment the start op finishes, and that connection runs the sync's probe:
// no `ssh true`, no second probe.
func TestRunStartedGuestFirstConnectionIsTheProbe(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{StartDelay: 300 * time.Millisecond})
	defer fake.Close()
	f := newRunFixture(t, fake)
	ctx := context.Background()
	if err := runRun(ctx, f.env, RunOptions{Name: testSlug, NoAttach: true}, false); err != nil {
		t.Fatalf("first runRun: %v", err)
	}
	projects, err := f.env.Client.ListProjects(ctx)
	if err != nil || len(projects) != 1 {
		t.Fatalf("projects: %v %+v", err, projects)
	}
	fake.SetState(projects[0].ID, "stopped")

	var buf bytes.Buffer
	prev := timingOut
	timingOut = &buf
	defer func() { timingOut = prev }()
	env2 := &Env{
		Dir: f.env.Dir, Cfg: f.env.Cfg, Cache: f.env.Cache, Cwd: f.local, HomeDir: f.env.HomeDir,
		Client: f.env.Client, Out: &discardWriter{}, ErrOut: &discardWriter{}, TargetFor: f.env.TargetFor,
	}
	if err := runRun(ctx, env2, RunOptions{NoAttach: true}, false); err != nil {
		t.Fatalf("second runRun: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"run: guest up; first connection started beside the project read",
		"connect: the connection made when the guest came up is the master",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ssh true ") {
		t.Fatalf("an `ssh true` ran after the boot connection:\n%s", out)
	}
	if n := len(regexp.MustCompile(`(?m)^repose-timing \+\d+ms ssh `).FindAllString(out, -1)); n > 2 {
		t.Fatalf("%d ssh commands (want the probe and at most the apply):\n%s", n, out)
	}
	p, err := f.env.Client.GetProject(ctx, projects[0].ID)
	if err != nil || p.State != "running" {
		t.Fatalf("project after run: %v %+v", err, p)
	}
}

// A boot connection that failed (the guest refused it) is never the
// master: connect closes it and proves the connection the usual way.
func TestBootProbeFailureFallsBack(t *testing.T) {
	e := &Env{TargetFor: func(string) sshTarget {
		return sshTarget{Args: []string{"-o", "ConnectTimeout=1", "-p", "1", "127.0.0.1"}}
	}}
	p := &Project{ID: "id-1", Slug: "s"}
	startBootProbe(context.Background(), e, p, true)
	ep := e.early
	if ep == nil || !ep.boot {
		t.Fatal("no boot probe")
	}
	<-ep.done
	if ep.err == nil {
		t.Fatal("a probe to a closed port succeeded")
	}
	if _, ok := ep.connected(context.Background(), e, p); ok {
		t.Fatal("a failed boot probe was taken as the master")
	}
	// The sync then runs its own probe: forProject hands over the error.
	if fp := ep.forProject(); fp != nil {
		if _, err := fp(); err == nil {
			t.Fatal("the failed probe's output was offered as good")
		}
	}
}

// startBootProbe waits out the gateway's route cache after an early
// probe was refused, instead of dialling into the cached refusal.
func TestBootProbeWaitsOutTheRouteCache(t *testing.T) {
	old := &earlyProbe{id: "id-1", slug: "s", cold: true, done: make(chan struct{}), err: errors.New("refused"), started: time.Now().Add(-gatewayRouteTTL - routeTTLMargin + 300*time.Millisecond)}
	close(old.done)
	var buf bytes.Buffer
	prev := timingOut
	timingOut = &buf
	defer func() { timingOut = prev }()
	e := &Env{early: old, TargetFor: func(string) sshTarget {
		return sshTarget{Args: []string{"-o", "ConnectTimeout=1", "-p", "1", "127.0.0.1"}}
	}}
	began := time.Now()
	startBootProbe(context.Background(), e, &Project{ID: "id-1", Slug: "s"}, false)
	<-e.early.done
	if el := time.Since(began); el < 250*time.Millisecond {
		t.Fatalf("dialled %s after the call; the route cache had 300 ms left", el)
	}
	if old.usable {
		t.Fatal("the refused early probe is still usable")
	}
	if !strings.Contains(buf.String(), "out its route cache") {
		t.Fatalf("timing: %s", buf.String())
	}
}
