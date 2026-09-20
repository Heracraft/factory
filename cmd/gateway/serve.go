package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/heracraft/repose/internal/gateway"
	"github.com/heracraft/repose/internal/gateway/preview"
	"github.com/heracraft/repose/internal/obs"
	obsmetrics "github.com/heracraft/repose/internal/obs/metrics"
)

// serve runs the SSH gateway and its side listeners until ctx is cancelled.
func serve(ctx context.Context) error {
	log := obs.NewLogger(obs.LogOptions{Component: obs.ComponentGateway, Level: logLevel()})
	reg := obsmetrics.NewVersion(obs.ComponentGateway, version)
	m := obsmetrics.NewGatewayMetrics(reg)

	api, err := apiClient()
	if err != nil {
		return err
	}
	host, err := hostSigner()
	if err != nil {
		return err
	}
	gwKey, err := gatewaySigner()
	if err != nil {
		return err
	}

	gw, err := gateway.New(gateway.Config{
		API:        api,
		HostKey:    host,
		GatewayKey: gwKey,
		GuestPort:  22,
		Log:        log,
		Metrics:    m,
	})
	if err != nil {
		return err
	}
	// Fetch the CA and revocation list before accepting; if the api is down
	// the gateway still binds and refuses connections with the documented
	// message until a refresh succeeds (§6).
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	if err := gw.Prime(pctx); err != nil {
		log.Warn("prime failed; starting with an empty cache", "event", "route_fail", "reason", "prime", "err", err.Error())
	}
	cancel()

	ln, err := net.Listen("tcp", env("GATEWAY_LISTEN", ":22"))
	if err != nil {
		return fmt.Errorf("gateway listen: %w", err)
	}
	log.Info("gateway listening", "event", "listen", "addr", ln.Addr().String())

	var wg sync.WaitGroup
	run := func(name string, fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}

	run("refresh", func() { gw.RefreshLoop(ctx) })
	run("serve", func() {
		if err := gw.Serve(ctx, ln); err != nil {
			log.Error("gateway serve stopped", "event", "route_fail", "reason", "serve", "err", err.Error())
		}
	})

	// Metrics on the WireGuard address only (docs/ops/OBSERVABILITY.md).
	if addr := os.Getenv("METRICS_LISTEN"); addr != "" {
		run("metrics", func() {
			if err := reg.Serve(ctx, addr); err != nil {
				log.Error("metrics server stopped", "event", "route_fail", "reason", "metrics", "err", err.Error())
			}
		})
	}

	// Hook ingest on the WireGuard address :8443 with the edge's internal
	// certificate; guests reach it only over WireGuard (§5.7).
	if addr := os.Getenv("HOOK_LISTEN"); addr != "" {
		hi := gateway.NewHookIngest(api, log, m)
		run("hook-ingest", func() {
			serveTLS(ctx, log, "hook-ingest", addr, os.Getenv("HOOK_TLS_CERT"), os.Getenv("HOOK_TLS_KEY"), hi.Handler())
		})
	}

	// Preview-proxy stub on :443 with the wildcard certificate (§5.8).
	if addr := env("PREVIEW_LISTEN", ":443"); os.Getenv("PREVIEW_TLS_CERT") != "" {
		run("preview", func() {
			serveTLS(ctx, log, "preview", addr, os.Getenv("PREVIEW_TLS_CERT"), os.Getenv("PREVIEW_TLS_KEY"), preview.Handler())
		})
	}

	<-ctx.Done()
	log.Info("gateway shutting down", "event", "session_close", "reason", "signal")
	wg.Wait()
	return nil
}

// serveTLS runs an HTTPS server that shuts down when ctx ends. A missing
// certificate is logged and the listener is skipped rather than failing the
// whole gateway, so the relay runs before workstream 11 has placed the certs.
func serveTLS(ctx context.Context, log *slog.Logger, name, addr, certFile, keyFile string, h http.Handler) {
	if certFile == "" || keyFile == "" {
		log.Warn("listener disabled: no certificate", "event", "listen", "listener", name)
		return
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	log.Info("listener up", "event", "listen", "listener", name, "addr", addr)
	if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("listener stopped", "event", "route_fail", "reason", name, "err", err.Error())
	}
}

func logLevel() slog.Level {
	switch os.Getenv("LOG_LEVEL") {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
