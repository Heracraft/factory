// Command fakeapi runs internal/fakes/api as a standalone process on a
// random localhost port, for the dashboard's Playwright suite to spawn
// (docs/workstreams/08-dashboard.md §7: "Playwright end to end against
// internal/fakes/api (Go, run as a test fixture on a port)").
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/heracraft/repose/internal/fakes/api"
)

func main() {
	billing := flag.Bool("billing", false, "enable the billing routes instead of 503 billing_disabled")
	flag.Parse()

	f := api.New(api.Options{Billing: *billing})
	defer f.Close()

	// The one line of stdout this process ever prints: the base url a
	// harness reads to configure PUBLIC_API_URL. Nothing else goes to
	// stdout so a line-reader never has to guess which line it is.
	fmt.Printf("FAKEAPI_URL=%s/v1\n", f.URL())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}
