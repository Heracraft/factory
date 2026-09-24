package cli

import "fmt"

// NotAvailableMessage is what `repose mcp forward` and `repose browser
// bridge` print: reserved command names that exit 0 rather than error,
// per 07-cli.md §2 and DESIGN.md §18/§11.
// The page named is the public docs' account of what works today; the
// design files it used to name are not something a user has (I-241).
func NotAvailableMessage(name string) string {
	page := "agents#mcp-servers"
	if name == "repose browser bridge" {
		page = "browser#things-that-dont-work-yet"
	}
	return fmt.Sprintf("%s is not available yet. https://repose.herakraft.co/docs/%s says what works today.", name, page)
}
