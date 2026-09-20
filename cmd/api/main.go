// Command api is the repose control plane (docs/workstreams/05-control-
// plane-api.md): HTTP for the CLI and dashboard, gRPC for hosts, the ops
// driver and the background jobs. API_MODE selects the Coolify app
// (DECISIONS I-2): `http`, `grpc`, or `all` for one process.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/heracraft/repose/internal/api/app"
)

var version = "dev" // set by -ldflags at release

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("api", version)
		return
	}
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	mode := fs.String("mode", "", "http, grpc or all (default $API_MODE or all)")
	migrate := fs.Bool("migrate", false, "apply pending migrations before serving")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	cfg, err := app.FromEnv()
	if *mode != "" {
		cfg.Mode = *mode
		err = cfg.Validate()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(2)
	}
	cfg.Migrate = *migrate
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	a, err := app.New(ctx, cfg, version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}
	if err := a.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}
}
