package cli

import (
	"strings"
	"testing"
)

// The reserved command points at the public docs, not at repository files
// a user does not have.
func TestNotAvailableMessage(t *testing.T) {
	msg := NotAvailableMessage("repose mcp forward")
	want := "https://repose.herakraft.co/docs/agents#mcp-servers"
	if !strings.Contains(msg, want) || strings.Contains(msg, "DESIGN.md") || strings.Contains(msg, "docs/features") {
		t.Errorf("repose mcp forward: %q, want it to name %s", msg, want)
	}
}
