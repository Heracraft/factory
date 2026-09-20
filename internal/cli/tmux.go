package cli

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const paneIdleWait = 1 * time.Second
const paneIdlePoll = 100 * time.Millisecond
const paneIdleTimeout = 30 * time.Second

// windowExists reports whether tmux session slug already has a window
// named name.
func windowExists(ctx context.Context, t sshTarget, slug, name string) (bool, error) {
	out, err := runSSH(ctx, t, fmt.Sprintf("tmux list-windows -t %s -F '#{window_name}'", slug), nil)
	if err != nil {
		return false, err
	}
	for _, w := range nonEmptyLines(string(out)) {
		if w == name {
			return true, nil
		}
	}
	return false, nil
}

// windowNameFor picks the agent's window name, appending "-2" if one
// already exists (07-cli.md §5.5 step 7 and §6's "second prompt while
// agent window exists").
func windowNameFor(ctx context.Context, t sshTarget, slug, agent string) (name string, alreadyExisted bool, err error) {
	exists, err := windowExists(ctx, t, slug, agent)
	if err != nil {
		return "", false, err
	}
	if !exists {
		return agent, false, nil
	}
	return agent + "-2", true, nil
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
// real binary name, tests substitute a stand-in.
func startAgentWindow(ctx context.Context, t sshTarget, slug, windowName, binary, prompt string, attachOnly bool) error {
	cmd := fmt.Sprintf("tmux new-window -t %s -n %s -c ~/%s -d %s", slug, windowName, slug, shQuote(binary))
	if _, err := runSSH(ctx, t, cmd, nil); err != nil {
		return err
	}
	if attachOnly {
		return nil
	}
	if err := waitPaneIdle(ctx, t, slug, windowName, binary); err != nil {
		return err
	}
	if _, err := runSSH(ctx, t, fmt.Sprintf("tmux send-keys -t %s:%s -l %s", slug, windowName, shQuote(prompt)), nil); err != nil {
		return err
	}
	_, err := runSSH(ctx, t, fmt.Sprintf("tmux send-keys -t %s:%s Enter", slug, windowName), nil)
	return err
}

// waitPaneIdle polls pane_current_command until it names binary and its
// captured content has not changed for paneIdleWait.
func waitPaneIdle(ctx context.Context, t sshTarget, slug, windowName, binary string) error {
	deadline := time.Now().Add(paneIdleTimeout)
	var lastCapture string
	var stableSince time.Time
	for {
		cmdOut, err := runSSH(ctx, t, fmt.Sprintf("tmux display -p -t %s:%s '#{pane_current_command}'", slug, windowName), nil)
		if err != nil {
			return err
		}
		current := strings.TrimSpace(string(cmdOut))
		capture, err := runSSH(ctx, t, fmt.Sprintf("tmux capture-pane -p -t %s:%s", slug, windowName), nil)
		if err != nil {
			return err
		}
		if current == binary {
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
