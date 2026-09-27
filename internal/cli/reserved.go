package cli

import "fmt"

// NotAvailableMessage is what `repose mcp forward` prints: a reserved
// command name that exits 0 rather than error, per 07-cli.md §2 and
// DESIGN.md §18. The page named is the public docs' account of what works
// today; the design files it used to name are not something a user has
// (I-241). `repose browser bridge` was reserved the same way until I-296
// built it.
func NotAvailableMessage(name string) string {
	return fmt.Sprintf("%s is not available yet. https://repose.herakraft.co/docs/agents#mcp-servers says what works today.", name)
}
