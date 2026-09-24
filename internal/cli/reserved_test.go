package cli

import (
	"strings"
	"testing"
)

// The reserved commands point at the public docs, not at repository files
// a user does not have.
func TestNotAvailableMessage(t *testing.T) {
	for name, want := range map[string]string{
		"repose mcp forward":    "https://repose.herakraft.co/docs/agents#mcp-servers",
		"repose browser bridge": "https://repose.herakraft.co/docs/machine#browser",
	} {
		msg := NotAvailableMessage(name)
		if !strings.Contains(msg, want) || strings.Contains(msg, "DESIGN.md") || strings.Contains(msg, "docs/features") {
			t.Errorf("%s: %q, want it to name %s", name, msg, want)
		}
	}
}
