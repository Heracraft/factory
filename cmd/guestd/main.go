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
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/heracraft/repose/internal/guestd"
	"github.com/heracraft/repose/internal/obs"
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
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Println("guestd", version, "protocol", guestd.ProtocolVersion)
			return nil
		case "call":
			return runCall(os.Args[2:])
		}
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

	level, err := obs.ParseLevel(*logLevel)
	if err != nil {
		return err
	}
	// Logs go to stderr, which is the serial console in a guest. Nothing is
	// written to a file, so logging keeps working while the root filesystem is
	// frozen for a snapshot.
	log := obs.NewLogger(obs.LogOptions{Component: obs.ComponentGuestd, Level: level, Writer: os.Stderr})

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
