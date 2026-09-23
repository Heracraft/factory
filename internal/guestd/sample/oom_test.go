package sample

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// DECISIONS I-213: the nix packages run the agents through makeWrapper,
// so claude's process is `.claude-wrapped` (comm and exe), and a longer
// name is cut to 15 bytes in comm. Each is still its window's agent, and
// protected; an unrelated dotted process is not.
func TestOOMPriorityFindsNixWrappedAgents(t *testing.T) {
	procs := []fakeProc{
		{pid: 100, ppid: 1, comm: "tmux: server", uid: 1000},
		{pid: 200, ppid: 100, comm: "bash", uid: 1000},
		{pid: 201, ppid: 200, comm: ".claude-wrapped", uid: 1000, exe: ".claude-wrapped"},
		{pid: 300, ppid: 100, comm: "bash", uid: 1000},
		{pid: 301, ppid: 300, comm: ".opencode-wrapp", uid: 1000},
		{pid: 400, ppid: 100, comm: "bash", uid: 1000},
		{pid: 401, ppid: 400, comm: ".codex-wrapped", uid: 1000},
		{pid: 500, ppid: 100, comm: "bash", uid: 1000},
		{pid: 501, ppid: 500, comm: ".claudeish", uid: 1000},
	}
	r, _ := newProcFixture(t, procs)
	children, ok := r.childIndex()
	if !ok {
		t.Fatal("no child index")
	}
	agents := r.agentPIDs(children, map[int]string{200: "claude", 300: "opencode", 400: "codex", 500: "claude"})
	if len(agents) != 3 || !agents[201] || !agents[301] || !agents[401] {
		t.Fatalf("agents = %v, want 201, 301 and 401", agents)
	}
	if !isAgentCommand("claude", ".claude-wrapped") || isAgentCommand("claude", ".claude") || isAgentCommand("pi", ".claude-wrapped") {
		t.Error("isAgentCommand does not follow nix's wrapped names")
	}
}

// I-200 on a fixture /proc: the tmux server and each window's agent get
// OOMProtected; everything of dev's that inherited a negative value (the
// pane's shell, an MCP server under claude, the vite Gemini started, which
// is `node` like Gemini itself) goes back to 0; root's processes and a
// value dev raised on purpose are left alone; a second pass writes nothing.
func TestOOMPriority(t *testing.T) {
	procs := []fakeProc{
		{pid: 100, ppid: 1, comm: "tmux: server", uid: 1000},
		{pid: 200, ppid: 100, comm: "bash", uid: 1000},
		{pid: 201, ppid: 200, comm: "claude", uid: 1000},
		{pid: 202, ppid: 201, comm: "node", uid: 1000, exe: "node"},
		{pid: 300, ppid: 100, comm: "bash", uid: 1000},
		{pid: 301, ppid: 300, comm: "MainThread", uid: 1000, exe: "node"},
		{pid: 302, ppid: 301, comm: "node", uid: 1000, exe: "node"},
		{pid: 400, ppid: 1, comm: "dockerd", uid: 0},
		{pid: 500, ppid: 100, comm: "bash", uid: 1000},
		{pid: 501, ppid: 500, comm: "python3", uid: 1000},
	}
	r, p := newProcFixture(t, procs)
	adj := map[int]int{100: 0, 200: -800, 201: -800, 202: -800, 300: -800, 301: -800, 302: -800, 400: -500, 500: -800, 501: 300}
	for pid, v := range adj {
		if err := os.WriteFile(filepath.Join(p.ProcPID(fmt.Sprint(pid)), "oom_score_adj"), []byte(fmt.Sprintf("%d\n", v)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	children, ok := r.childIndex()
	if !ok {
		t.Fatal("no child index")
	}
	agents := r.agentPIDs(children, map[int]string{200: "claude", 300: "gemini"})
	if len(agents) != 2 || !agents[201] || !agents[301] {
		t.Fatalf("agents = %v, want 201 (claude) and 301 (gemini's node, not the vite under it)", agents)
	}
	changes := r.applyOOM(1000, agents)
	got := map[int]int{}
	for pid := range adj {
		b, _ := os.ReadFile(filepath.Join(p.ProcPID(fmt.Sprint(pid)), "oom_score_adj"))
		var v int
		_, _ = fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &v)
		got[pid] = v
	}
	want := map[int]int{100: -800, 200: 0, 201: -800, 202: 0, 300: 0, 301: -800, 302: 0, 400: -500, 500: 0, 501: 300}
	for pid, w := range want {
		if got[pid] != w {
			t.Errorf("pid %d oom_score_adj = %d, want %d", pid, got[pid], w)
		}
	}
	t.Logf("changes: %+v", changes)
	if again := r.applyOOM(1000, agents); len(again) != 0 {
		t.Errorf("second pass wrote %+v", again)
	}
}
