package sample

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Agents outlive dev servers under memory pressure (DECISIONS I-200).
// oom_score_adj is inherited on fork, so a value set once on an agent
// would also protect every dev server the agent starts; and the `dev`
// user can raise its own value but never lower it. So guestd (root) owns
// it and re-applies it on every refresh: OOMProtected for the tmux server
// and for each agent window's agent process, and 0 for any other process
// of dev's that inherited a negative value, so a vite an agent started is
// back to the kernel's ordinary choice within one refresh. Nothing is
// ever killed or stopped by guestd; the kernel's OOM killer decides, and
// the existing `oom` warning names what it killed.
//
// Only process names, stat fields, the uid and the exe link's basename are
// read, never a command line (DECISIONS R5-3).

// OOMProtected is the value agents and the tmux server run with: strongly
// preferred to survive, but not -1000 (never killable), so a runaway agent
// can still be stopped by the kernel rather than hang the guest.
const OOMProtected = -800

// tmuxServerComm is the name tmux's server process gives itself.
const tmuxServerComm = "tmux: server"

// agentPIDs returns, for each agent window's pane, the agent's own
// process: the shallowest process on each branch of the pane's tree whose
// name or executable is one of the agent's binaries. The agent's children
// are not in it, even when they share its name (a `node` dev server under
// Gemini CLI, which is itself `node`).
func (r *procReader) agentPIDs(children map[int][]int, panes map[int]string) map[int]bool {
	out := map[int]bool{}
	for root, agent := range panes {
		wants := binaries[agent]
		stack := []int{root}
		seen := map[int]bool{}
		for len(stack) > 0 {
			pid := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[pid] {
				continue
			}
			seen[pid] = true
			comm, _, _, ok := r.readStat(strconv.Itoa(pid))
			if ok && (matchesBinary(wants, comm) || matchesBinary(wants, r.readExeBase(pid))) {
				out[pid] = true
				continue // its descendants are its work, not the agent
			}
			stack = append(stack, children[pid]...)
		}
	}
	return out
}

// oomChange is one write applyOOM made, for tests and the debug log.
type oomChange struct {
	PID  int
	Comm string
	From int
	To   int
}

// applyOOM sets oom_score_adj on dev's processes: OOMProtected for the
// tmux server and the agents, 0 for any other that has a negative value.
// A positive value the user chose (choom) is left alone. It writes only
// what differs.
func (r *procReader) applyOOM(devUID int, agents map[int]bool) []oomChange {
	entries, err := os.ReadDir(r.paths.Proc())
	if err != nil {
		return nil
	}
	var changes []oomChange
	for _, e := range entries {
		if !isPID(e.Name()) {
			continue
		}
		dir := r.paths.ProcPID(e.Name())
		if procUID(filepath.Join(dir, "status")) != devUID {
			continue
		}
		comm, _, _, ok := r.readStat(e.Name())
		if !ok {
			continue
		}
		pid, _ := strconv.Atoi(e.Name())
		adjPath := filepath.Join(dir, "oom_score_adj")
		b, err := os.ReadFile(adjPath)
		if err != nil {
			continue
		}
		cur, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil {
			continue
		}
		want := cur
		switch {
		case agents[pid] || comm == tmuxServerComm:
			want = OOMProtected
		case cur < 0:
			want = 0
		}
		if want == cur {
			continue
		}
		if err := os.WriteFile(adjPath, []byte(strconv.Itoa(want)), 0o644); err != nil {
			continue // exited, or not ours to change; the next refresh tries again
		}
		changes = append(changes, oomChange{PID: pid, Comm: comm, From: cur, To: want})
	}
	return changes
}
