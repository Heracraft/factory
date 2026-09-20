package cli

import "fmt"

// NotAvailableMessage is what `repose mcp forward` and `repose browser
// bridge` print: reserved command names that exit 0 rather than error,
// per 07-cli.md §2 and DESIGN.md §18/§11.
func NotAvailableMessage(name string) string {
	return fmt.Sprintf("%s is not available yet. See docs/features/agents.md and docs/DESIGN.md §18 for the plan.", name)
}
