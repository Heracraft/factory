// Command repose-admin is the operator CLI (DECISIONS I-9, docs/ops/
// RUNBOOK.md). It talks to Postgres directly and enqueues ops the
// api-grpc process drives. Run it as an exec into the api container.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/heracraft/repose/internal/admin"
)

var version = "dev" // set by -ldflags at release

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("repose-admin", version)
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	err := admin.Run(ctx, &admin.Env{Stdout: os.Stdout, Stderr: os.Stderr}, os.Args[1:])
	if err == nil {
		return
	}
	if errors.Is(err, admin.ErrUsage) {
		fmt.Fprintln(os.Stderr, "repose-admin:", err)
		fmt.Fprint(os.Stderr, admin.Usage)
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, "repose-admin:", err)
	os.Exit(1)
}
