// Command repose is the CLI a developer installs. See
// docs/workstreams/07-cli.md for the workstream that owns it and
// docs/interfaces/ for the contracts it implements.
package main

import (
	"os"

	"github.com/heracraft/repose/internal/cli"
)

var version = "dev" // set by -ldflags at release

func main() {
	os.Exit(cli.Execute(version))
}
