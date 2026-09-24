package cli

import "testing"

// --agent and config.toml's default_agent accept exactly the five agents
// every guest ships (features/run-and-attach.md); anything else is refused
// before a tmux window is opened for it.
func TestIsAgent(t *testing.T) {
	for _, a := range []string{"claude", "opencode", "codex", "gemini", "pi"} {
		if !isAgent(a) {
			t.Errorf("isAgent(%q) = false", a)
		}
	}
	for _, a := range []string{"", "Claude", "bash", "claude-2", "aider"} {
		if isAgent(a) {
			t.Errorf("isAgent(%q) = true", a)
		}
	}
}
