// Package net wires a guest into the host network per
// docs/interfaces/host-conventions.md: a tap on br-guests, membership in
// the nftables `guests` set, a per-guest egress counter and rule in the
// hostd-owned chain `guest_dyn`, and an HTB class shaping egress.
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
	AddGuestRules(ctx context.Context, guestID, ip, tap string) error
	DelGuestRules(ctx context.Context, guestID, ip, tap string) error
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

// Real drives ip, nft and tc through a shell.Runner.
type Real struct {
	Bridge string // br-guests
	Family string // inet
	Table  string // repose
	Chain  string // guest_dyn
	Set    string // guests
	SysFS  string // /sys/class/net
	R      shell.Runner
}

// NewReal returns the documented defaults.
func NewReal(r shell.Runner) *Real {
	return &Real{Bridge: "br-guests", Family: "inet", Table: "repose", Chain: "guest_dyn", Set: "guests", SysFS: "/sys/class/net", R: r}
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

// AddTap implements Net.
func (n *Real) AddTap(ctx context.Context, tap string) error {
	ok, err := n.TapExists(ctx, tap)
	if err != nil {
		return err
	}
	if !ok {
		if _, err := n.R.Run(ctx, "ip", "tuntap", "add", "dev", tap, "mode", "tap"); err != nil {
			return err
		}
	}
	_, err = n.R.Run(ctx, "ip", "link", "set", "dev", tap, "master", n.Bridge, "up")
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

// AddGuestRules implements Net.
func (n *Real) AddGuestRules(ctx context.Context, guestID, ip, tap string) error {
	if _, err := n.R.Run(ctx, "nft", "add", "element", n.Family, n.Table, n.Set, "{ "+ip+" . "+tap+" }"); err != nil {
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

// DelGuestRules implements Net: set element, the rule (found by handle) and
// nothing else; the counter stays until DelCounter reads its final value.
func (n *Real) DelGuestRules(ctx context.Context, guestID, ip, tap string) error {
	_, err := n.R.Run(ctx, "nft", "delete", "element", n.Family, n.Table, n.Set, "{ "+ip+" . "+tap+" }")
	var ee *shell.ExitError
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

// Shape implements Net with an HTB root and one class.
func (n *Real) Shape(ctx context.Context, tap string, mbit int) error {
	if _, err := n.R.Run(ctx, "tc", "qdisc", "replace", "dev", tap, "root", "handle", "1:", "htb", "default", "10"); err != nil {
		return err
	}
	rate := strconv.Itoa(mbit) + "mbit"
	_, err := n.R.Run(ctx, "tc", "class", "replace", "dev", tap, "parent", "1:", "classid", "1:10", "htb", "rate", rate, "ceil", rate)
	return err
}

// Unshape implements Net.
func (n *Real) Unshape(ctx context.Context, tap string) error {
	_, err := n.R.Run(ctx, "tc", "qdisc", "del", "dev", tap, "root")
	var ee *shell.ExitError
	if errors.As(err, &ee) {
		return nil // no qdisc, or no device: both mean nothing to remove
	}
	return err
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
	Elements map[string]string // ip . tap by guest
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

func (f *Fake) AddGuestRules(_ context.Context, guestID, ip, tap string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Ops = append(f.Ops, "rules+"+guestID)
	if err := f.FailOn["rules"]; err != nil {
		return err
	}
	f.Elements[guestID] = ip + " . " + tap
	if _, ok := f.Counters[guestID]; !ok {
		f.Counters[guestID] = 0
	}
	return nil
}

func (f *Fake) DelGuestRules(_ context.Context, guestID, _, _ string) error {
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
