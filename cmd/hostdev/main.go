// Command hostdev is the one-host dev driver (DECISIONS I-17): it plays
// the api side of docs/interfaces/grpc-hostd.md for exactly one host so
// the M1 gate is reached before the api exists, and stays as the
// break-glass tool for a host that has lost the api.
package main

import (
	"fmt"
	"os"

	"github.com/heracraft/repose/internal/hostdev"
)

var version = "dev" // set by -ldflags at release

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("hostdev", version)
		return
	}
	os.Exit(hostdev.Main(os.Args[1:]))
}
