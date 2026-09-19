package sample

import (
	"context"
	"strconv"
	"strings"

	"github.com/heracraft/repose/internal/guestd/sysdep"
)

// Agents are the window names that mean an agent, from
// docs/interfaces/guest-conventions.md. A second instance of an agent gets
// "<agent>-2", which agentOf also recognises.
var Agents = []string{"claude", "opencode", "codex", "gemini", "pi"}

// binaries maps an agent to the process name to look for in the pane's
// process tree (features/agents.md's "Binary" column).
var binaries = map[string]string{
	"claude":   "claude",
	"opencode": "opencode",
	"codex":    "codex",
	"gemini":   "gemini",
	"pi":       "pi",
}

// HookedAgents are the agents whose completion is reported by a real hook.
// The rest fall back to the pane-idle heuristic, and the notification says so
// (features/agents.md: "claude finished" versus "gemini went idle").
var HookedAgents = map[string]bool{"claude": true, "codex": true, "opencode": true}

// AgentOf maps a tmux window name to an agent name, or "" if the window is not
// an agent window. "claude" and "claude-2" are both claude.
func AgentOf(window string) string {
	for _, a := range Agents {
		if window == a || strings.HasPrefix(window, a+"-") {
			rest := strings.TrimPrefix(window, a+"-")
			if window == a {
				return a
			}
			if rest != "" && allDigits(rest) {
				return a
			}
		}
	}
	return ""
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

// tmuxWindow is one row of `tmux list-windows`.
type tmuxWindow struct {
	Name         string
	PanePID      int
	PaneCommand  string
	LastActivity int64 // unix seconds, 0 when tmux did not report it
}

// tmuxClient runs tmux as dev. tmux is the only thing guestd forks for on the
// sampling path, which is why the watcher caches the result.
type tmuxClient struct {
	paths sysdep.Paths
	run   sysdep.Runner
}

// tmuxFailure classifies a non-zero tmux exit into a bounded reason, so the
// failure is logged without the message, which carries the project slug.
// Silently reporting "no windows" for any of these is how a guest looks idle
// while an agent is working in it.
func tmuxFailure(stderr string) string {
	switch {
	case strings.Contains(stderr, "no server running"):
		return "server_down"
	case strings.Contains(stderr, "can't find session"), strings.Contains(stderr, "session not found"):
		return "session_missing"
	case strings.Contains(stderr, "error connecting"), strings.Contains(stderr, "Permission denied"):
		return "connect_failed"
	case strings.Contains(stderr, "not found"), strings.Contains(stderr, "No such file"):
		return "tmux_missing"
	default:
		return "other"
	}
}

// listWindows returns the windows of the project's session, and whether a
// tmux server is running at all.
func (t tmuxClient) listWindows(ctx context.Context, session string) ([]tmuxWindow, bool, error) {
	const format = "#{window_name}\t#{pane_pid}\t#{pane_current_command}\t#{window_activity}"
	res, err := t.run.Run(ctx, sysdep.RunSpec{
		Argv:      []string{"tmux", "list-windows", "-t", session, "-F", format},
		User:      "dev",
		Env:       sysdep.DevEnv(t.paths, "dev"),
		MaxOutput: 32 << 10,
	})
	if err != nil {
		return nil, false, sysdep.Errf(sysdep.CodeInternal, "list tmux windows: %w", err)
	}
	if res.ExitCode != 0 {
		reason := tmuxFailure(string(res.Stderr))
		if reason == "server_down" {
			return nil, false, nil
		}
		if reason == "session_missing" {
			// The project is not set up yet; that is a state, not a fault.
			return nil, true, nil
		}
		return nil, true, sysdep.Errf(sysdep.CodeInternal,
			"list tmux windows: tmux exited %d (%s)", res.ExitCode, reason)
	}
	var out []tmuxWindow
	for _, line := range strings.Split(strings.TrimRight(string(res.Stdout), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		pid, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		w := tmuxWindow{Name: parts[0], PanePID: pid, PaneCommand: parts[2]}
		if len(parts) > 3 {
			if ts, err := strconv.ParseInt(parts[3], 10, 64); err == nil {
				w.LastActivity = ts
			}
		}
		out = append(out, w)
	}
	return out, true, nil
}

// listClients counts attached tmux clients for the session.
func (t tmuxClient) listClients(ctx context.Context, session string) (uint32, bool, error) {
	res, err := t.run.Run(ctx, sysdep.RunSpec{
		Argv:      []string{"tmux", "list-clients", "-t", session, "-F", "#{client_tty}"},
		User:      "dev",
		Env:       sysdep.DevEnv(t.paths, "dev"),
		MaxOutput: 8 << 10,
	})
	if err != nil {
		return 0, false, sysdep.Errf(sysdep.CodeInternal, "list tmux clients: %w", err)
	}
	if res.ExitCode != 0 {
		reason := tmuxFailure(string(res.Stderr))
		if reason == "server_down" {
			return 0, false, nil
		}
		if reason == "session_missing" {
			return 0, true, nil
		}
		return 0, true, sysdep.Errf(sysdep.CodeInternal,
			"list tmux clients: tmux exited %d (%s)", res.ExitCode, reason)
	}
	var n uint32
	for _, line := range strings.Split(strings.TrimRight(string(res.Stdout), "\n"), "\n") {
		if line != "" {
			n++
		}
	}
	return n, true, nil
}

// windowOfPane resolves a tmux pane id (the $TMUX_PANE of a hook's caller) to
// its window name.
func (t tmuxClient) windowOfPane(ctx context.Context, pane string) (string, error) {
	res, err := t.run.Run(ctx, sysdep.RunSpec{
		Argv:      []string{"tmux", "display-message", "-p", "-t", pane, "#{window_name}"},
		User:      "dev",
		Env:       sysdep.DevEnv(t.paths, "dev"),
		MaxOutput: 4 << 10,
	})
	if err != nil {
		return "", sysdep.Errf(sysdep.CodeInternal, "resolve tmux pane: %w", err)
	}
	if res.ExitCode != 0 {
		return "", sysdep.NotFound("resolve tmux pane: tmux exited %d", res.ExitCode)
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}
