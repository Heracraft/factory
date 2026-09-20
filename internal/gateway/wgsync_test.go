package gateway

import (
	"context"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
	"github.com/heracraft/repose/internal/obs"
	obsmetrics "github.com/heracraft/repose/internal/obs/metrics"
)

// fakeWG records peer and route operations and answers Peers from its
// current set, so a sync's diff can be asserted.
type fakeWG struct {
	mu     sync.Mutex
	peers  map[string][]string // pubkey -> allowed-ips
	routes map[string]bool     // cidr -> present
	setErr map[string]error
}

func newFakeWG(initial ...string) *fakeWG {
	w := &fakeWG{peers: map[string][]string{}, routes: map[string]bool{}, setErr: map[string]error{}}
	for _, p := range initial {
		w.peers[p] = nil
	}
	return w
}

func (w *fakeWG) Peers(context.Context) ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, 0, len(w.peers))
	for p := range w.peers {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

func (w *fakeWG) SetPeer(_ context.Context, pubkey string, allowed []string, _ time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.setErr[pubkey]; err != nil {
		return err
	}
	w.peers[pubkey] = allowed
	return nil
}

func (w *fakeWG) RemovePeer(_ context.Context, pubkey string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.peers, pubkey)
	return nil
}

func (w *fakeWG) ReplaceRoute(_ context.Context, cidr string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.routes[cidr] = true
	return nil
}

func (w *fakeWG) DeleteRoute(_ context.Context, cidr string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.routes, cidr)
	return nil
}

func (w *fakeWG) snapshot() (map[string][]string, map[string]bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := map[string][]string{}
	for k, v := range w.peers {
		p[k] = v
	}
	r := map[string]bool{}
	for k := range w.routes {
		r[k] = true
	}
	return p, r
}

func TestWGSyncAddsAndRemovesPeers(t *testing.T) {
	api := fakeapi.New(fakeapi.Options{})
	defer api.Close()
	client, err := NewClient(api.URL(), nil, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	wg := newFakeWG("staleoldpeerkeythatisnolongerinthelist000000=")
	m := obsmetrics.NewGatewayMetrics(obsmetrics.New(obs.ComponentGateway))
	s := NewWGSync(client, wg, "wg0", nil, m)

	// Two registered hosts.
	api.SetHosts([]fakeapi.Host{
		{HostID: "h1", WGPubkey: "peer1key00000000000000000000000000000000000=", WGIP: "10.255.0.7", GuestCIDR: "10.64.4.0/22", State: "ready"},
		{HostID: "h2", WGPubkey: "peer2key00000000000000000000000000000000000=", WGIP: "10.255.0.8", GuestCIDR: "10.64.8.0/22", State: "ready"},
	})
	if err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	peers, routes := wg.snapshot()
	if len(peers) != 2 {
		t.Fatalf("peers after sync: %v", peers)
	}
	if got := peers["peer1key00000000000000000000000000000000000="]; strings.Join(got, ",") != "10.255.0.7/32,10.64.4.0/22" {
		t.Fatalf("peer1 allowed-ips %v", got)
	}
	if !routes["10.64.4.0/22"] || !routes["10.64.8.0/22"] {
		t.Fatalf("routes %v", routes)
	}
	if counterValue(t, m.WGSyncPeers) != 2 {
		t.Fatalf("wgsync_peers = %v", counterValue(t, m.WGSyncPeers))
	}

	// A host is retired and another removed from the list: its peer and
	// route go away within one sync.
	api.SetHosts([]fakeapi.Host{
		{HostID: "h1", WGPubkey: "peer1key00000000000000000000000000000000000=", WGIP: "10.255.0.7", GuestCIDR: "10.64.4.0/22", State: "ready"},
		{HostID: "h2", WGPubkey: "peer2key00000000000000000000000000000000000=", WGIP: "10.255.0.8", GuestCIDR: "10.64.8.0/22", State: "retired"},
	})
	if err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	peers, routes = wg.snapshot()
	if len(peers) != 1 || peers["peer1key00000000000000000000000000000000000="] == nil {
		t.Fatalf("after retire, peers %v", peers)
	}
	if routes["10.64.8.0/22"] {
		t.Fatalf("retired host's route still present: %v", routes)
	}
	if counterValue(t, m.WGSyncPeers) != 1 {
		t.Fatalf("wgsync_peers = %v", counterValue(t, m.WGSyncPeers))
	}
}

func TestParsePeers(t *testing.T) {
	out := "peerAkey=\npeerBkey=\n\n"
	got := parsePeers(out)
	if len(got) != 2 || got[0] != "peerAkey=" || got[1] != "peerBkey=" {
		t.Fatalf("parsePeers = %v", got)
	}
}

func TestExecWGBuildsCommands(t *testing.T) {
	var cmds [][]string
	e := &execWG{iface: "wg0", run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		cmds = append(cmds, append([]string{name}, args...))
		if name == "wg" && len(args) > 1 && args[0] == "show" {
			return []byte("peerXkey=\n"), nil
		}
		return nil, nil
	}}
	ctx := context.Background()
	peers, err := e.Peers(ctx)
	if err != nil || len(peers) != 1 {
		t.Fatalf("peers %v %v", peers, err)
	}
	if err := e.SetPeer(ctx, "peerXkey=", []string{"10.255.0.7/32", "10.64.4.0/22"}, 25*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := e.ReplaceRoute(ctx, "10.64.4.0/22"); err != nil {
		t.Fatal(err)
	}
	if err := e.RemovePeer(ctx, "peerXkey="); err != nil {
		t.Fatal(err)
	}
	want := "wg set wg0 peer peerXkey= allowed-ips 10.255.0.7/32,10.64.4.0/22 persistent-keepalive 25"
	if got := strings.Join(cmds[1], " "); got != want {
		t.Fatalf("set peer command:\n got %q\nwant %q", got, want)
	}
	if got := strings.Join(cmds[2], " "); got != "ip route replace 10.64.4.0/22 dev wg0" {
		t.Fatalf("route command: %q", got)
	}
	if got := strings.Join(cmds[3], " "); got != "wg set wg0 peer peerXkey= remove" {
		t.Fatalf("remove command: %q", got)
	}
	// A malformed CIDR is refused before shelling out.
	if err := e.ReplaceRoute(ctx, "not-a-cidr"); err == nil {
		t.Fatal("ReplaceRoute accepted a bad CIDR")
	}
}
