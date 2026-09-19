package sample

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/guestd/sysdep"
)

func TestSampleCombinesCachedSignalsAndAFreshProcWalk(t *testing.T) {
	procs := []fakeProc{
		{pid: 100, ppid: 1, comm: "bash"},
		{pid: 101, ppid: 100, comm: "claude", ticks: 50},
		{pid: 102, ppid: 1, comm: "sshd", uid: 1000},
		{pid: 103, ppid: 1, comm: "node", ticks: 900},
	}
	w, run, _, clk, _ := newWatcherFixture(t, procs)
	run.Match["list-windows"] = tmuxOutput([4]string{"claude", "101", "claude", "0"})
	run.Match["list-clients"] = sysdep.RunResult{Stdout: []byte("/dev/pts/0\n/dev/pts/1\n")}
	ctx := context.Background()
	w.Refresh(ctx)

	h := NewHandler(w.paths, w, quietLog(), clk.now)
	res, err := h.Sample(ctx)
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	sig := res.GetSignals()
	if sig.GetSshSessions() != 1 {
		t.Errorf("ssh_sessions = %d, want 1", sig.GetSshSessions())
	}
	if sig.GetTmuxClients() != 2 {
		t.Errorf("tmux_clients = %d, want 2", sig.GetTmuxClients())
	}
	if sig.GetDockerContainers() != 2 {
		t.Errorf("docker_containers = %d, want 2", sig.GetDockerContainers())
	}
	if !sig.GetGuestdOk() {
		t.Error("guestd_ok is false in guestd's own sample")
	}
	if len(sig.GetAgents()) != 1 || sig.GetAgents()[0].GetAgent() != "claude" {
		t.Errorf("agents = %+v", sig.GetAgents())
	}
	if len(res.GetProcs()) == 0 {
		t.Error("no process samples")
	}
	if res.GetPartial() {
		t.Error("partial was set on a healthy sample")
	}
}

func TestSampleNeverCarriesArgumentsOrPaths(t *testing.T) {
	w, run, _, clk, _ := newWatcherFixture(t, []fakeProc{
		{pid: 100, ppid: 1, comm: "node", ticks: 5},
	})
	run.Match["list-windows"] = tmuxOutput()
	w.Refresh(context.Background())

	h := NewHandler(w.paths, w, quietLog(), clk.now)
	res, err := h.Sample(context.Background())
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	// The ProcSample shape has one string field and it is the process name.
	// This is the wire-level half of the privacy promise; the strace test is
	// the other half.
	for _, p := range res.GetProcs() {
		if p.GetComm() == "" {
			t.Error("a sample has no name")
		}
		if len(p.GetComm()) > 64 {
			t.Errorf("comm %q is longer than a kernel comm can be, which suggests a command line", p.GetComm())
		}
	}
}

func TestSampleReportsPartialOnAStaleCache(t *testing.T) {
	w, run, _, clk, _ := newWatcherFixture(t, nil)
	run.Match["list-windows"] = tmuxOutput()
	w.Refresh(context.Background())
	clk.advance(CacheStale + time.Second)

	h := NewHandler(w.paths, w, quietLog(), clk.now)
	res, err := h.Sample(context.Background())
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	if !res.GetPartial() {
		t.Fatal("a stale cache must come back as partial, not as current-looking zeroes")
	}
}

func TestSampleIsUnder20Milliseconds(t *testing.T) {
	// A guest of a few hundred processes, which is a busy one.
	var procs []fakeProc
	for i := 0; i < 300; i++ {
		procs = append(procs, fakeProc{pid: 1000 + i, ppid: 1, comm: fmt.Sprintf("proc%03d", i), ticks: uint64(i)})
	}
	w, run, _, _, _ := newWatcherFixture(t, procs)
	run.Match["list-windows"] = tmuxOutput()
	ctx := context.Background()
	w.Refresh(ctx)

	h := NewHandler(w.paths, w, quietLog(), time.Now)
	if _, err := h.Sample(ctx); err != nil { // prime the CPU deltas
		t.Fatalf("sample: %v", err)
	}

	const runs = 20
	start := time.Now()
	for i := 0; i < runs; i++ {
		if _, err := h.Sample(ctx); err != nil {
			t.Fatalf("sample %d: %v", i, err)
		}
	}
	avg := time.Since(start) / runs
	t.Logf("sample of %d processes took %v on average", len(procs), avg)
	if avg > 20*time.Millisecond {
		t.Fatalf("a sample took %v, over the 20 ms budget in docs/workstreams/04-guestd.md", avg)
	}
}
