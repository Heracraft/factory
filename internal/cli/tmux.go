package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

// agentNames are the five agents every guest ships (docs/features/agents.md).
var agentNames = []string{"claude", "opencode", "codex", "gemini", "pi"}

func isAgent(name string) bool {
	return slices.Contains(agentNames, name)
}

const paneIdleWait = 1 * time.Second
const paneIdlePoll = 100 * time.Millisecond

// paneIdleTimeout is a variable so a test can show a load outlasting it.
var paneIdleTimeout = 30 * time.Second

// devShellLoadTimeout bounds how long startAgentWindow keeps waiting while
// the agent wrapper loads the checkout's dev environment, which it marks
// with the pane option devShellLoadingOption (DECISIONS I-259). A first
// load builds the dev shell, which can take minutes, and a prompt typed
// before the agent runs is lost.
const devShellLoadTimeout = 30 * time.Minute

// devShellLoadingOption is the tmux pane option the agent wrapper
// (nix/overlay/agents/devshell.sh) sets to "loading" while it loads the
// dev environment and unsets after (guest-conventions.md "Agent wrappers").
const devShellLoadingOption = "@repose-devshell"

// listWindows returns the names of tmux session slug's windows.
func listWindows(ctx context.Context, t sshTarget, slug string) ([]string, error) {
	out, err := runSSH(ctx, t, fmt.Sprintf("tmux list-windows -t %s -F '#{window_name}'", slug), nil)
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(string(out)), nil
}

// isWindowOf reports whether window is one of agent's windows: the agent's
// own name or "<agent>-N" (the rule guestd's sample.AgentOf applies).
func isWindowOf(agent, window string) bool {
	if window == agent {
		return true
	}
	n, ok := strings.CutPrefix(window, agent+"-")
	if !ok || n == "" {
		return false
	}
	for _, c := range n {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// nextWindowName is the lowest free name among agent, agent-2, agent-3, ...
// (DECISIONS I-253: any number of agent windows in one guest).
func nextWindowName(agent string, taken func(string) bool) string {
	if !taken(agent) {
		return agent
	}
	for n := 2; ; n++ {
		if name := fmt.Sprintf("%s-%d", agent, n); !taken(name) {
			return name
		}
	}
}

// windowNameFor picks the agent's window name: the agent's own name, else
// the lowest free "<agent>-N" (07-cli.md §5.5 step 7 and §6's "second
// prompt while agent window exists", DECISIONS I-253). othersOpen says
// another window of the same agent is open, which is what the
// shared-working-tree warning is for.
func windowNameFor(ctx context.Context, t sshTarget, slug, agent string) (name string, othersOpen bool, err error) {
	windows, err := listWindows(ctx, t, slug)
	if err != nil {
		return "", false, err
	}
	name, othersOpen = pickWindow(agent, windows, nil)
	return name, othersOpen, nil
}

// pickWindow is windowNameFor's choice over a known window list;
// alsoTaken names what a worktree run must skip besides open windows.
func pickWindow(agent string, windows []string, alsoTaken func(string) bool) (name string, othersOpen bool) {
	open := map[string]bool{}
	for _, w := range windows {
		open[w] = true
		if isWindowOf(agent, w) {
			othersOpen = true
		}
	}
	name = nextWindowName(agent, func(n string) bool {
		return open[n] || (alsoTaken != nil && alsoTaken(n))
	})
	return name, othersOpen
}

// needsClaudeLogin implements the check in 07-cli.md §5.5 step 7: the
// claude agent with no on-guest credentials file and no
// CLAUDE_CODE_OAUTH_TOKEN secret must be attached to, not sent a prompt.
func needsClaudeLogin(ctx context.Context, t sshTarget, hasOAuthSecret bool) (bool, error) {
	if hasOAuthSecret {
		return false, nil
	}
	// Exists-check on the guest only; the file is never read or copied
	// (DECISIONS R2-8).
	err := runSSHOK(ctx, t, "test -f ~/.claude/.credentials.json")
	return err != nil, nil // a non-zero test means the file is missing
}

// startAgentWindow opens the tmux window and, unless the caller says to
// attach instead, waits for the pane to go idle and sends the prompt.
// binary is the process tmux launches and the name pane_current_command
// must settle on before the prompt is sent; production passes the agent's
// real binary name, tests substitute a stand-in. dir is the window's
// working directory as the guest's shell spells it: "~/<slug>", or a
// worktree's "~/<slug>-<window>" (I-253). onLoading, when not nil, is
// called once if the wrapper says it is loading the dev environment.
func startAgentWindow(ctx context.Context, t sshTarget, slug, windowName, dir, binary, prompt string, attachOnly bool, onLoading func()) error {
	cmd := fmt.Sprintf("tmux new-window -t %s -n %s -c %s -d %s", slug, windowName, dir, shQuote(binary))
	if _, err := runSSH(ctx, t, cmd, nil); err != nil {
		return err
	}
	if attachOnly {
		return nil
	}
	if err := waitPaneIdle(ctx, t, slug, windowName, binary, onLoading); err != nil {
		return err
	}
	if _, err := runSSH(ctx, t, fmt.Sprintf("tmux send-keys -t %s:%s -l %s", slug, windowName, shQuote(prompt)), nil); err != nil {
		return err
	}
	_, err := runSSH(ctx, t, fmt.Sprintf("tmux send-keys -t %s:%s Enter", slug, windowName), nil)
	return err
}

// waitPaneIdle polls pane_current_command until it names binary and its
// captured content has not changed for paneIdleWait. While the pane
// carries devShellLoadingOption the agent has not started yet, and the
// wait goes on past paneIdleTimeout, up to devShellLoadTimeout (I-259).
func waitPaneIdle(ctx context.Context, t sshTarget, slug, windowName, binary string, onLoading func()) error {
	start := time.Now()
	deadline := start.Add(paneIdleTimeout)
	var lastCapture string
	var stableSince time.Time
	sawLoading := false
	for {
		cmdOut, err := runSSH(ctx, t, fmt.Sprintf("tmux display -p -t %s:%s '#{pane_current_command} #{%s}'", slug, windowName, devShellLoadingOption), nil)
		if err != nil {
			return err
		}
		current, marker, _ := strings.Cut(strings.TrimSpace(string(cmdOut)), " ")
		loading := strings.TrimSpace(marker) == "loading"
		capture, err := runSSH(ctx, t, fmt.Sprintf("tmux capture-pane -p -t %s:%s", slug, windowName), nil)
		if err != nil {
			return err
		}
		if loading {
			stableSince = time.Time{}
			if !sawLoading && onLoading != nil {
				onLoading()
			}
			sawLoading = true
			deadline = time.Now().Add(paneIdleTimeout)
			if limit := start.Add(devShellLoadTimeout); deadline.After(limit) {
				deadline = limit
			}
		} else if current == binary {
			if string(capture) == lastCapture {
				if !stableSince.IsZero() && time.Since(stableSince) >= paneIdleWait {
					return nil
				}
				if stableSince.IsZero() {
					stableSince = time.Now()
				}
			} else {
				stableSince = time.Time{}
			}
		} else {
			stableSince = time.Time{}
		}
		lastCapture = string(capture)
		if time.Now().After(deadline) {
			return nil // best effort: send the prompt anyway rather than hang forever
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(paneIdlePoll):
		}
	}
}

// capturePane is a small helper the integration test uses to assert the
// prompt landed inside the agent's pane.
func capturePane(ctx context.Context, t sshTarget, slug, windowName string) (string, error) {
	out, err := runSSH(ctx, t, fmt.Sprintf("tmux capture-pane -p -t %s:%s", slug, windowName), nil)
	return string(out), err
}

// shQuote single-quotes s for a POSIX shell command line, the way the
// doc's literal `tmux send-keys -l '<prompt>'` needs when prompt itself
// may contain spaces or shell metacharacters.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// agentWorktree is where a `repose run --worktree` agent works (DECISIONS
// I-253): a git worktree of the guest's checkout beside it, outside the
// synced tree, on its own branch.
type agentWorktree struct {
	Window string // tmux window name, "<agent>" or "<agent>-N"
	Dir    string // "~/<slug>-<window>", as the guest's shell spells it
	Branch string // "repose/<window>"
	Base   string // the checkout's HEAD the branch starts from
	Dirty  bool   // the checkout had uncommitted changes, which the worktree lacks
}

// worktreeDir and worktreeBranch are the only places the worktree layout
// is spelled (DECISIONS I-253, interfaces/guest-conventions.md).
func worktreeDir(slug, window string) string { return "~/" + slug + "-" + window }
func worktreeBranch(window string) string    { return "repose/" + window }

// worktreeProbeScript reports, in one ssh, the session's windows, the
// checkout's HEAD and whether it is dirty, and which worktree directories
// and repose/ branches already exist, so the window name skips all three.
func worktreeProbeScript(slug, agent string) string {
	return fmt.Sprintf(`set -e
tmux list-windows -t %[1]s -F '#window #{window_name}'
cd ~/%[1]s 2>/dev/null && [ -e .git ] || { echo '#nogit'; exit 0; }
h=$(git rev-parse -q --verify HEAD) || { echo '#nohead'; exit 0; }
echo "#head $h"
[ -z "$(git status --porcelain 2>/dev/null)" ] || echo '#dirty'
git for-each-ref --format='#branch %%(refname:strip=3)' refs/heads/repose/
for p in ~/%[1]s-%[2]s ~/%[1]s-%[2]s-*; do [ -e "$p" ] && echo "#dir ${p##*/}"; done
true`, slug, agent)
}

// prepareWorktree picks the window name for a worktree run and creates the
// worktree: `git worktree add -b repose/<window> ~/<slug>-<window> HEAD`.
// It never reuses a worktree: a name whose window, directory or branch
// exists is skipped, so each --worktree run starts fresh from HEAD.
func prepareWorktree(ctx context.Context, t sshTarget, slug, agent string) (*agentWorktree, error) {
	out, err := runSSH(ctx, t, worktreeProbeScript(slug, agent), nil)
	if err != nil {
		return nil, stepFailed("list the guest's tmux windows", err, "")
	}
	var windows []string
	taken := map[string]bool{}
	wt := &agentWorktree{}
	for _, l := range nonEmptyLines(string(out)) {
		switch l {
		case "#nogit":
			return nil, exitf(ExitUsage, "--worktree needs a git checkout in the guest, and ~/%s is not one. Run without --worktree.", slug)
		case "#nohead":
			return nil, exitf(ExitUsage, "~/%s in the guest has no commits yet, so there is nothing to start a worktree from. Commit first, or run without --worktree.", slug)
		case "#dirty":
			wt.Dirty = true
		default:
			if w, ok := strings.CutPrefix(l, "#window "); ok {
				windows = append(windows, w)
			} else if h, ok := strings.CutPrefix(l, "#head "); ok {
				wt.Base = h
			} else if b, ok := strings.CutPrefix(l, "#branch "); ok {
				taken["branch "+b] = true
			} else if d, ok := strings.CutPrefix(l, "#dir "); ok {
				taken["dir "+d] = true
			}
		}
	}
	wt.Window, _ = pickWindow(agent, windows, func(n string) bool {
		return taken["branch "+n] || taken["dir "+slug+"-"+n]
	})
	wt.Dir = worktreeDir(slug, wt.Window)
	wt.Branch = worktreeBranch(wt.Window)
	cmd := fmt.Sprintf("git -C ~/%s worktree add -q -b %s %s %s", slug, shQuote(wt.Branch), wt.Dir, wt.Base)
	if _, err := runSSH(ctx, t, cmd, nil); err != nil {
		return nil, stepFailed("create the worktree "+wt.Dir+" in the guest", err, "")
	}
	return wt, nil
}
