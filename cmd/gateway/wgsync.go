package main

import (
	"context"

	"github.com/heracraft/repose/internal/gateway"
	"github.com/heracraft/repose/internal/obs"
	obsmetrics "github.com/heracraft/repose/internal/obs/metrics"
)

// wgsyncMain runs the WireGuard peer reconciler until ctx is cancelled. It
// is a separate process (and systemd unit) so its CAP_NET_ADMIN is not held
// by the relay (nix/edge, §5.6).
func wgsyncMain(ctx context.Context) error {
	log := obs.NewLogger(obs.LogOptions{Component: obs.ComponentGateway, Level: logLevel()})
	m := obsmetrics.NewGatewayMetrics(obsmetrics.NewVersion(obs.ComponentGateway, version))
	api, err := apiClient()
	if err != nil {
		return err
	}
	iface := env("WG_INTERFACE", "wg0")
	s := gateway.NewWGSync(api, gateway.NewExecWG(iface), iface, log, m)
	log.Info("wgsync starting", "event", "wgsync", "iface", iface)
	s.Run(ctx)
	return nil
}
