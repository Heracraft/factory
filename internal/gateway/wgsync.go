package gateway

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"sort"
	"strings"
	"time"

	obsmetrics "github.com/heracraft/repose/internal/obs/metrics"
)

// WGSyncInterval is how often the edge reconciles WireGuard peers from the
// api's host list (docs/workstreams/06-gateway-edge.md §5.6).
const WGSyncInterval = 30 * time.Second

// WG is the slice of WireGuard and routing operations wgsync needs. The
// real implementation shells out to `wg` and `ip`; tests supply a fake.
type WG interface {
	// Peers lists the public keys currently configured on the interface.
	Peers(ctx context.Context) ([]string, error)
	// SetPeer adds or updates a peer with its allowed IPs and keepalive.
	SetPeer(ctx context.Context, pubkey string, allowedIPs []string, keepalive time.Duration) error
	// RemovePeer removes a peer by public key.
	RemovePeer(ctx context.Context, pubkey string) error
	// ReplaceRoute makes cidr reachable over the interface.
	ReplaceRoute(ctx context.Context, cidr string) error
	// DeleteRoute removes a route added for a peer that is gone.
	DeleteRoute(ctx context.Context, cidr string) error
}

// WGSync reconciles the interface's peers with the api's host list.
type WGSync struct {
	api    *Client
	wg     WG
	iface  string
	log    *slog.Logger
	m      *obsmetrics.GatewayMetrics
	clock  func() time.Time
	routes map[string]string // pubkey -> guest cidr, for removing routes of departed hosts
}

// NewWGSync builds a syncer for the interface (usually wg0).
func NewWGSync(api *Client, wg WG, iface string, log *slog.Logger, m *obsmetrics.GatewayMetrics) *WGSync {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &WGSync{api: api, wg: wg, iface: iface, log: log, m: m, clock: time.Now, routes: map[string]string{}}
}

// Run reconciles every WGSyncInterval until ctx ends.
func (s *WGSync) Run(ctx context.Context) {
	t := time.NewTicker(WGSyncInterval)
	defer t.Stop()
	if err := s.Sync(ctx); err != nil {
		s.log.Warn("wgsync failed", "event", "route_fail", "reason", "wgsync", "err", err.Error())
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.Sync(ctx); err != nil {
				s.log.Warn("wgsync failed", "event", "route_fail", "reason", "wgsync", "err", err.Error())
			}
		}
	}
}

// Sync performs one reconciliation: it applies every non-retired host as a
// peer with its wg address and guest CIDR, and removes peers no longer in
// the list. It is idempotent and logs the diff.
func (s *WGSync) Sync(ctx context.Context) error {
	hosts, err := s.api.Hosts(ctx)
	if err != nil {
		if s.m != nil {
			s.m.WGSyncErrorsTotal.Inc()
		}
		return fmt.Errorf("host list: %w", err)
	}
	want := map[string]Host{}
	for _, h := range hosts {
		if h.State == "retired" || h.WGPubkey == "" || h.WGIP == "" || h.GuestCIDR == "" {
			continue
		}
		want[h.WGPubkey] = h
	}
	current, err := s.wg.Peers(ctx)
	if err != nil {
		if s.m != nil {
			s.m.WGSyncErrorsTotal.Inc()
		}
		return fmt.Errorf("wg peers: %w", err)
	}
	have := map[string]bool{}
	for _, p := range current {
		have[p] = true
	}

	var added, removed, failed int
	// Add or update every wanted peer.
	for pubkey, h := range want {
		allowed := []string{h.WGIP + "/32", h.GuestCIDR}
		if err := s.wg.SetPeer(ctx, pubkey, allowed, 25*time.Second); err != nil {
			s.log.Warn("wgsync set peer failed", "event", "route_fail", "reason", "wg_set", "host_id", h.HostID, "err", err.Error())
			failed++
			continue
		}
		if err := s.wg.ReplaceRoute(ctx, h.GuestCIDR); err != nil {
			s.log.Warn("wgsync route failed", "event", "route_fail", "reason", "ip_route", "host_id", h.HostID, "err", err.Error())
			failed++
			continue
		}
		s.routes[pubkey] = h.GuestCIDR
		if !have[pubkey] {
			added++
			s.log.Info("wgsync added peer", "event", "wgsync", "host_id", h.HostID)
		}
	}
	// Remove peers that are no longer wanted.
	for _, pubkey := range current {
		if _, ok := want[pubkey]; ok {
			continue
		}
		if err := s.wg.RemovePeer(ctx, pubkey); err != nil {
			s.log.Warn("wgsync remove peer failed", "event", "route_fail", "reason", "wg_remove", "err", err.Error())
			failed++
			continue
		}
		if cidr := s.routes[pubkey]; cidr != "" {
			if err := s.wg.DeleteRoute(ctx, cidr); err != nil {
				s.log.Warn("wgsync delete route failed", "event", "route_fail", "reason", "ip_route_del", "err", err.Error())
			}
			delete(s.routes, pubkey)
		}
		removed++
		s.log.Info("wgsync removed peer", "event", "wgsync")
	}
	if s.m != nil {
		s.m.WGSyncPeers.Set(float64(len(want)))
		if failed > 0 {
			s.m.WGSyncErrorsTotal.Inc()
		}
	}
	if added > 0 || removed > 0 {
		s.log.Info("wgsync reconciled", "event", "wgsync", "peers", len(want), "added", added, "removed", removed)
	}
	if failed > 0 {
		return fmt.Errorf("wgsync: %d operations failed", failed)
	}
	return nil
}

// execWG is the production WG that shells out to `wg` and `ip`.
type execWG struct {
	iface string
	run   func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// NewExecWG returns a WG driving the interface with the wg and ip binaries.
func NewExecWG(iface string) WG {
	return &execWG{iface: iface, run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}}
}

func (e *execWG) Peers(ctx context.Context) ([]string, error) {
	out, err := e.run(ctx, "wg", "show", e.iface, "peers")
	if err != nil {
		return nil, fmt.Errorf("wg show %s peers: %v: %s", e.iface, err, strings.TrimSpace(string(out)))
	}
	return parsePeers(string(out)), nil
}

// parsePeers reads the base64 public keys from `wg show <iface> peers`, one
// per line.
func parsePeers(out string) []string {
	var peers []string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			peers = append(peers, line)
		}
	}
	sort.Strings(peers)
	return peers
}

func (e *execWG) SetPeer(ctx context.Context, pubkey string, allowedIPs []string, keepalive time.Duration) error {
	args := []string{"set", e.iface, "peer", pubkey,
		"allowed-ips", strings.Join(allowedIPs, ","),
		"persistent-keepalive", fmt.Sprintf("%d", int(keepalive.Seconds()))}
	if out, err := e.run(ctx, "wg", args...); err != nil {
		return fmt.Errorf("wg set peer: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (e *execWG) RemovePeer(ctx context.Context, pubkey string) error {
	if out, err := e.run(ctx, "wg", "set", e.iface, "peer", pubkey, "remove"); err != nil {
		return fmt.Errorf("wg remove peer: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (e *execWG) ReplaceRoute(ctx context.Context, cidr string) error {
	if _, _, err := net.ParseCIDR(cidr); err != nil {
		return fmt.Errorf("route %q: %w", cidr, err)
	}
	if out, err := e.run(ctx, "ip", "route", "replace", cidr, "dev", e.iface); err != nil {
		return fmt.Errorf("ip route replace: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (e *execWG) DeleteRoute(ctx context.Context, cidr string) error {
	if out, err := e.run(ctx, "ip", "route", "del", cidr, "dev", e.iface); err != nil {
		return fmt.Errorf("ip route del: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
