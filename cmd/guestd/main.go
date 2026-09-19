// Command guestd is the repose in-guest daemon. It serves the protocol of
// docs/interfaces/vsock-guestd.md on vsock port 5000 and the agent hook socket
// at /run/repose/hooks.sock, and it has no network listener.
//
// Workstream: docs/workstreams/04-guestd.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/heracraft/repose/internal/guestd"
	"github.com/heracraft/repose/internal/vsockrpc"
)

var version = "dev" // set by -ldflags at release

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "guestd:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("guestd", version, "protocol", guestd.ProtocolVersion)
		return nil
	}

	fs := flag.NewFlagSet("guestd", flag.ContinueOnError)
	var (
		devSocket = fs.String("dev-socket", "", "serve the protocol on this unix socket instead of vsock")
		root      = fs.String("root", "", "prefix every path with this directory (tests and development)")
		port      = fs.Uint("vsock-port", uint(vsockrpc.Port), "vsock port to listen on")
		hookSock  = fs.String("hook-socket", "", "override the agent hook socket path")
		logLevel  = fs.String("log-level", "info", "debug, info, warn or error")
	)
	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	// guestd must not compete with the agent for the guest's two to eight
	// vCPUs; one thread is more than it needs (docs/workstreams/04-guestd.md).
	if _, set := os.LookupEnv("GOMAXPROCS"); !set {
		runtime.GOMAXPROCS(1)
	}

	level, err := parseLevel(*logLevel)
	if err != nil {
		return err
	}
	// Logs go to stderr, which is the serial console in a guest. Nothing is
	// written to a file, so logging keeps working while the root filesystem is
	// frozen for a snapshot.
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.String("ts", a.Value.Time().UTC().Format(time.RFC3339))
			}
			if a.Key == slog.MessageKey {
				return slog.String("msg", a.Value.String())
			}
			return a
		},
	}))

	srv, err := guestd.New(guestd.Config{
		Root:       *root,
		DevSocket:  *devSocket,
		VsockPort:  uint32(*port),
		HookSocket: *hookSock,
		Log:        log,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return srv.Run(ctx)
}

func parseLevel(s string) (slog.Level, error) {
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown log level %q; use debug, info, warn or error", s)
	}
}
