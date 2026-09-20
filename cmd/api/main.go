// Command api is the repose control plane (docs/workstreams/05-control-
// plane-api.md): HTTP for the CLI and dashboard, gRPC for hosts, the ops
// driver and the background jobs. API_MODE selects the Coolify app
// (DECISIONS I-2): `http`, `grpc`, or `all` for one process.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	migrate := fs.Bool("migrate", false, "apply pending migrations before serving (also $API_MIGRATE, default on)")
	healthcheck := fs.Bool("healthcheck", false, "probe this container's own listener and exit 0 when healthy (compose healthcheck; the image has no shell or curl)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	cfg, err := app.FromEnv()
	if *healthcheck {
		os.Exit(probe(cfg))
	}
	if *mode != "" {
		cfg.Mode = *mode
		err = cfg.Validate()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(2)
	}
	if *migrate {
		cfg.Migrate = true
	}
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

// probe is the compose healthcheck (ops/coolify/docker-compose.yml). The
// runtime image is distroless, so the check has to be the binary itself.
// http and all: GET /healthz on the API listener, which also reports
// pending migrations as unhealthy. grpc: the process has no plain HTTP
// endpoint but the metrics listener, so a 200 from /metrics is the check;
// TLS and the CA are covered by the api's own logs and the hostd stream
// alert (docs/ops/OBSERVABILITY.md), not by this probe.
func probe(cfg app.Config) int {
	addr, path := cfg.Listen, "/healthz"
	if cfg.Mode == "grpc" {
		addr, path = cfg.MetricsListen, "/metrics"
	}
	if addr == "" || addr == "off" {
		fmt.Fprintln(os.Stderr, "healthcheck: no listener to probe in mode", cfg.Mode)
		return 1
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	if host == "" {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort(host, port) + path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}
