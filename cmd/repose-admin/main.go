// Command repose-admin is part of repose. See docs/workstreams/ for the workstream
// that owns it and docs/interfaces/ for the contracts it implements.
package main

import (
	"fmt"
	"os"
)

var version = "dev" // set by -ldflags at release

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("repose-admin", version)
		return
	}
	fmt.Fprintln(os.Stderr, "repose-admin: not implemented yet; see docs/workstreams/")
	os.Exit(2)
}
