// Package net wires a guest into the host network per
// docs/interfaces/host-conventions.md "Network": a tap on br-guests attached
// `isolated on learning off flood off` with a static FDB entry, membership
// in the `bridge repose` table's `guests` set (mac . ip . tap, which is what
// admits the guest's ARP and IPv4 frames to the host at all), a per-guest
// egress counter and rule in the hostd-owned `inet repose` chain
// `guest_dyn`, and a policer on the tap's ingress limiting what the guest
// sends (DECISIONS I-217). DECISIONS I-18 explains why
// the admission lives in the bridge family: frames between two taps never
// traverse the inet forward hook, and `learning off` plus the static FDB
// entry is what stops a guest claiming another guest's MAC.
package net

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/heracraft/repose/internal/hostd/shell"
)

// Net is what the guest state machine needs from the network layer.
type Net interface {
	TapExists(ctx context.Context, tap string) (bool, error)
	AddTap(ctx context.Context, tap string) error
	DelTap(ctx context.Context, tap string) error
	// AddGuestRules registers the guest's (mac, ip, tap) tuple with the
	// bridge (static FDB entry, `guests` set element) and creates its egress
	// counter and rule.
	AddGuestRules(ctx context.Context, guestID, ip, mac, tap string) error
	DelGuestRules(ctx context.Context, guestID, ip, mac, tap string) error
	Shape(ctx context.Context, tap string, mbit int) error
	Unshape(ctx context.Context, tap string) error
	// CounterBytes reads the guest's egress counter.
	CounterBytes(ctx context.Context, guestID string) (uint64, error)
	// DelCounter removes the counter after its final value was read.
	DelCounter(ctx context.Context, guestID string) error
	// TapStats returns bytes from the guest's point of view: rx is what the
	// guest received (the tap's tx), tx what it sent (the tap's rx).
	TapStats(tap string) (rx, tx uint64, err error)
	// ListTaps returns the repose taps present on the host.
	ListTaps(ctx context.Context) ([]string, error)
}

// CounterName is the nft counter for a guest.
func CounterName(guestID string) string { return "egress-" + guestID }

// Real drives ip, bridge, nft and tc through a shell.Runner.
type Real struct {
	Bridge       string // br-guests
	TapUser      string // hostd: the tap's owner (host-conventions.md)
	Family       string // inet: the table holding guest_dyn and the counters
	Table        string // repose
	Chain        string // guest_dyn
	BridgeFamily string // bridge: the table holding the guests set
	BridgeTable  string // repose
	Set          string // guests
	SysFS        string // /sys/class/net
	R            shell.Runner
}

// NewReal returns the documented defaults.
func NewReal(r shell.Runner) *Real {
	return &Real{Bridge: "br-guests", TapUser: "hostd", Family: "inet", Table: "repose", Chain: "guest_dyn", BridgeFamily: "bridge", BridgeTable: "repose", Set: "guests", SysFS: "/sys/class/net", R: r}
}

// TapExists implements Net.
func (n *Real) TapExists(ctx context.Context, tap string) (bool, error) {
	_, err := n.R.Run(ctx, "ip", "link", "show", "dev", tap)
	var ee *shell.ExitError
	if errors.As(err, &ee) {
		return false, nil
	}
	return err == nil, err
}

// AddTap implements Net with exactly the attach sequence the host's
// bridge table depends on: `isolated on` stops frames between taps at the
// bridge, `learning off` (with the static FDB entry AddGuestRules adds)
// stops a guest from claiming another guest's MAC, `flood off` keeps
// broadcast off every other tap. vnet_hdr is what the runner contract in
// guest-conventions.md expects of the tap.
func (n *Real) AddTap(ctx context.Context, tap string) error {
	ok, err := n.TapExists(ctx, tap)
	if err != nil {
		return err
	}
	if !ok {
		if _, err := n.R.Run(ctx, "ip", "tuntap", "add", "dev", tap, "mode", "tap", "user", n.TapUser, "vnet_hdr"); err != nil {
			return err
		}
	}
	if _, err := n.R.Run(ctx, "ip", "link", "set", "dev", tap, "master", n.Bridge, "up"); err != nil {
		return err
	}
	_, err = n.R.Run(ctx, "bridge", "link", "set", "dev", tap, "isolated", "on", "learning", "off", "flood", "off")
	return err
}

// DelTap implements Net.
func (n *Real) DelTap(ctx context.Context, tap string) error {
	ok, err := n.TapExists(ctx, tap)
	if err != nil || !ok {
		return err
	}
	_, err = n.R.Run(ctx, "ip", "link", "del", "dev", tap)
	return err
}

func (n *Real) counterExists(ctx context.Context, guestID string) (bool, error) {
	_, err := n.R.Run(ctx, "nft", "list", "counter", n.Family, n.Table, CounterName(guestID))
	var ee *shell.ExitError
	if errors.As(err, &ee) {
		return false, nil
	}
	return err == nil, err
}

func (n *Real) element(ip, mac, tap string) string {
	return "{ " + mac + " . " + ip + " . " + tap + " }"
}

// AddGuestRules implements Net. `bridge fdb replace` and `nft add element`
// are both idempotent, so a re-run after a crash converges.
func (n *Real) AddGuestRules(ctx context.Context, guestID, ip, mac, tap string) error {
	if _, err := n.R.Run(ctx, "bridge", "fdb", "replace", mac, "dev", tap, "master", "static"); err != nil {
		return err
	}
	if _, err := n.R.Run(ctx, "nft", "add", "element", n.BridgeFamily, n.BridgeTable, n.Set, n.element(ip, mac, tap)); err != nil {
		return err
	}
	ok, err := n.counterExists(ctx, guestID)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	if _, err := n.R.Run(ctx, "nft", "add", "counter", n.Family, n.Table, CounterName(guestID)); err != nil {
		return err
	}
	_, err = n.R.Run(ctx, "nft", "add", "rule", n.Family, n.Table, n.Chain, "ip", "saddr", ip, "counter", "name", `"`+CounterName(guestID)+`"`)
	return err
}

var handleRe = regexp.MustCompile(`counter name "([^"]+)".*# handle (\d+)`)

// DelGuestRules implements Net: set element, FDB entry, the rule (found by
// handle) and nothing else; the counter stays until DelCounter reads its
// final value. An element or entry that is already gone is not an error.
func (n *Real) DelGuestRules(ctx context.Context, guestID, ip, mac, tap string) error {
	_, err := n.R.Run(ctx, "nft", "delete", "element", n.BridgeFamily, n.BridgeTable, n.Set, n.element(ip, mac, tap))
	var ee *shell.ExitError
	if err != nil && !errors.As(err, &ee) {
		return err
	}
	_, err = n.R.Run(ctx, "bridge", "fdb", "del", mac, "dev", tap, "master")
	if err != nil && !errors.As(err, &ee) {
		return err
	}
	res, err := n.R.Run(ctx, "nft", "-a", "list", "chain", n.Family, n.Table, n.Chain)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		m := handleRe.FindStringSubmatch(line)
		if m != nil && m[1] == CounterName(guestID) {
			if _, err := n.R.Run(ctx, "nft", "delete", "rule", n.Family, n.Table, n.Chain, "handle", m[2]); err != nil {
				return err
			}
		}
	}
	return nil
}

// ShapeExempt are the destinations a guest's traffic is never policed
// toward: the host's guest ranges (the gateway, so DNS and whatever else
// the host answers on it; guest-to-guest is dropped by nftables anyway)
// and the host services address the caches listen on (nix/hosts/caches.nix,
// DECISIONS I-202). Both are fixed on every host (host-conventions.md
// "Network"), and traffic to them never leaves the host.
var ShapeExempt = []string{"10.64.0.0/12", "10.63.255.254/32"}

var (
	ingressRe    = regexp.MustCompile(`(?m)^qdisc ingress ffff: `)
	legacyRootRe = regexp.MustCompile(`(?m)^qdisc htb 1: root `)
)

// Shape implements Net with a policer on the tap's ingress, which is what
// the guest sends (DECISIONS I-217): on the ingress qdisc, one `pass`
// filter per ShapeExempt prefix, then a flower with no match policing every other IPv4
// packet to mbit with a 500 ms burst. What the host sends to the guest
// (downloads, the caches) is not limited.
//
// Idempotent and reconciling: the ingress qdisc is added only when absent
// and every filter is a `replace` with a fixed prio and handle, so a re-run
// or a new rate swaps them in place, with no window and no duplicate. A tap
// still carrying the root HTB of the shape before I-217 (which limited
// host-to-guest traffic) has it removed after the policer is in place;
// connections through it survive.
func (n *Real) Shape(ctx context.Context, tap string, mbit int) error {
	res, err := n.R.Run(ctx, "tc", "qdisc", "show", "dev", tap)
	if err != nil {
		return err
	}
	qd := string(res.Stdout)
	if !ingressRe.MatchString(qd) {
		if _, err := n.R.Run(ctx, "tc", "qdisc", "add", "dev", tap, "handle", "ffff:", "ingress"); err != nil {
			return err
		}
	}
	prio := 1
	for _, dst := range ShapeExempt {
		if _, err := n.R.Run(ctx, "tc", "filter", "replace", "dev", tap, "parent", "ffff:", "protocol", "ip",
			"prio", strconv.Itoa(prio), "handle", "1", "flower", "dst_ip", dst, "action", "pass"); err != nil {
			return err
		}
		prio++
	}
	rate := strconv.Itoa(mbit) + "mbit"
	burst := strconv.Itoa(mbit * 1000 * 1000 / 8 / 2) // 500 ms at the rate, in bytes
	// mtu 64kb: a vnet_hdr tap hands the host GSO packets of up to 64 KB,
	// which the policer's small default would count as exceeding.
	if _, err := n.R.Run(ctx, "tc", "filter", "replace", "dev", tap, "parent", "ffff:", "protocol", "ip",
		"prio", strconv.Itoa(prio), "handle", "1", "flower",
		"action", "police", "rate", rate, "burst", burst, "mtu", "64kb", "conform-exceed", "drop/ok"); err != nil {
		return err
	}
	if legacyRootRe.MatchString(qd) {
		if _, err := n.R.Run(ctx, "tc", "qdisc", "del", "dev", tap, "root"); err != nil {
			return err
		}
	}
	return nil
}

// Unshape implements Net: the ingress qdisc with its filters and, on a tap
// shaped before I-217, the root HTB. An exit status means no such qdisc or
// no device, both nothing to remove.
func (n *Real) Unshape(ctx context.Context, tap string) error {
	for _, dir := range []string{"ingress", "root"} {
		_, err := n.R.Run(ctx, "tc", "qdisc", "del", "dev", tap, dir)
		var ee *shell.ExitError
		if err != nil && !errors.As(err, &ee) {
			return err
		}
	}
	return nil
}

var bytesRe = regexp.MustCompile(`packets\s+\d+\s+bytes\s+(\d+)`)

// CounterBytes implements Net.
func (n *Real) CounterBytes(ctx context.Context, guestID string) (uint64, error) {
	res, err := n.R.Run(ctx, "nft", "list", "counter", n.Family, n.Table, CounterName(guestID))
	if err != nil {
		return 0, err
	}
	m := bytesRe.FindSubmatch(res.Stdout)
	if m == nil {
		return 0, fmt.Errorf("nft: no counter value in output for %s", guestID)
	}
	return strconv.ParseUint(string(m[1]), 10, 64)
}

// DelCounter implements Net.
func (n *Real) DelCounter(ctx context.Context, guestID string) error {
	_, err := n.R.Run(ctx, "nft", "delete", "counter", n.Family, n.Table, CounterName(guestID))
	var ee *shell.ExitError
	if errors.As(err, &ee) {
		return nil
	}
	return err
}

func readUint(path string) (uint64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
}

// TapStats implements Net.
func (n *Real) TapStats(tap string) (uint64, uint64, error) {
	d := filepath.Join(n.SysFS, tap, "statistics")
	tapTX, err := readUint(filepath.Join(d, "tx_bytes"))
	if err != nil {
		return 0, 0, err
	}
	tapRX, err := readUint(filepath.Join(d, "rx_bytes"))
	if err != nil {
		return 0, 0, err
	}
	return tapTX, tapRX, nil
}

// ListTaps implements Net.
func (n *Real) ListTaps(ctx context.Context) ([]string, error) {
	res, err := n.R.Run(ctx, "ip", "-o", "link", "show")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		name := strings.TrimSuffix(f[1], ":")
		if i := strings.Index(name, "@"); i >= 0 {
			name = name[:i]
		}
		if strings.HasPrefix(name, "tap-") {
			out = append(out, name)
		}
	}
	return out, nil
}

// Fake is the in-memory model for the state machine tests.
type Fake struct {
	mu       sync.Mutex
	Taps     map[string]bool
	Shaped   map[string]int
	Elements map[string]string // mac . ip . tap by guest
	Counters map[string]uint64
	Stats    map[string][2]uint64 // tap -> rx, tx (guest view)
	FailOn   map[string]error     // "tap", "rules", "shape"
	Ops      []string
}

// NewFake returns an empty fake network.
func NewFake() *Fake {
	return &Fake{Taps: map[string]bool{}, Shaped: map[string]int{}, Elements: map[string]string{}, Counters: map[string]uint64{}, Stats: map[string][2]uint64{}, FailOn: map[string]error{}}
}

func (f *Fake) TapExists(_ context.Context, tap string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Taps[tap], nil
}

func (f *Fake) AddTap(_ context.Context, tap string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Ops = append(f.Ops, "tap+"+tap)
	if err := f.FailOn["tap"]; err != nil {
		return err
	}
	f.Taps[tap] = true
	return nil
}

func (f *Fake) DelTap(_ context.Context, tap string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Ops = append(f.Ops, "tap-"+tap)
	delete(f.Taps, tap)
	delete(f.Shaped, tap)
	return nil
}

func (f *Fake) AddGuestRules(_ context.Context, guestID, ip, mac, tap string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Ops = append(f.Ops, "rules+"+guestID)
	if err := f.FailOn["rules"]; err != nil {
		return err
	}
	f.Elements[guestID] = mac + " . " + ip + " . " + tap
	if _, ok := f.Counters[guestID]; !ok {
		f.Counters[guestID] = 0
	}
	return nil
}

func (f *Fake) DelGuestRules(_ context.Context, guestID, _, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Ops = append(f.Ops, "rules-"+guestID)
	delete(f.Elements, guestID)
	return nil
}

func (f *Fake) Shape(_ context.Context, tap string, mbit int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Ops = append(f.Ops, "shape+"+tap)
	if err := f.FailOn["shape"]; err != nil {
		return err
	}
	f.Shaped[tap] = mbit
	return nil
}

func (f *Fake) Unshape(_ context.Context, tap string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Ops = append(f.Ops, "shape-"+tap)
	delete(f.Shaped, tap)
	return nil
}

func (f *Fake) CounterBytes(_ context.Context, guestID string) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.Counters[guestID]
	if !ok {
		return 0, fmt.Errorf("nft: counter %s missing", CounterName(guestID))
	}
	return v, nil
}

func (f *Fake) DelCounter(_ context.Context, guestID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Ops = append(f.Ops, "counter-"+guestID)
	delete(f.Counters, guestID)
	return nil
}

func (f *Fake) TapStats(tap string) (uint64, uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.Stats[tap]
	if !ok {
		return 0, 0, fmt.Errorf("no such tap %s", tap)
	}
	return s[0], s[1], nil
}

func (f *Fake) ListTaps(context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for t := range f.Taps {
		out = append(out, t)
	}
	return out, nil
}

// Leftovers reports anything still wired for a guest, for the rollback tests.
func (f *Fake) Leftovers(guestID, tap string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	if f.Taps[tap] {
		out = append(out, "tap")
	}
	if _, ok := f.Shaped[tap]; ok {
		out = append(out, "tc")
	}
	if _, ok := f.Elements[guestID]; ok {
		out = append(out, "nft element")
	}
	return out
}
