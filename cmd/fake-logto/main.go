// Command fake-logto runs test/fake-logto as a standalone process, for the
// dashboard's Playwright suite to sign in against instead of the real
// self-hosted Logto (docs/workstreams/08-dashboard.md §7).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	fakelogto "github.com/heracraft/repose/test/fake-logto"
)

func main() {
	f, err := fakelogto.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake-logto:", err)
		os.Exit(1)
	}
	defer f.Close()

	// The one line of stdout this process ever prints.
	fmt.Printf("FAKELOGTO_URL=%s\n", f.Issuer())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}
