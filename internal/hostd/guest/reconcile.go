package guest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/heracraft/repose/internal/hostd/state"
	"github.com/heracraft/repose/internal/hostd/virtiofs"
)

// Reconcile aligns bbolt with the host at start: a guest recorded as
// running whose unit is gone becomes stopped, a guest mid-transition is
// settled by what its unit says, running guests get their monitors back,
// and leftovers of stopped guests are torn down. Units for guests bbolt
// does not know are logged as orphans and adopted from guest.json when
// one exists.
func (m *Manager) Reconcile(ctx context.Context) error {
	units, err := m.d.Systemd.ListUnits(ctx, "guest@*")
	if err != nil {
		return err
	}
	active := map[string]bool{}
	for _, u := range units {
		id := strings.TrimSuffix(strings.TrimPrefix(u, "guest@"), ".service")
		active[id] = true
	}
	gs, err := m.d.State.ListGuests()
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, g := range gs {
		known[g.GuestID] = true
		m.reconcileGuest(ctx, g, active[g.GuestID])
	}
	for id := range active {
		if known[id] {
			continue
		}
		m.d.Log.Warn("orphan guest unit", "event", "reconcile_orphan", "guest_id", id)
		if g := m.guestFromDisk(id); g != nil {
			if _, err := m.d.State.AllocIndex(g.GuestID, m.maxIndex); err == nil {
				_ = m.d.State.PutGuest(g) // adopted from guest.json; a failed write is retried at the next reconcile
				m.reconcileGuest(ctx, g, true)
			}
		}
	}
	m.sweepStaleSnapshots(ctx)
	m.refreshGuestGauge()
	return nil
}

// sweepStaleSnapshots removes LVM snapshot volumes left by a hostd that
// died between `lvcreate -s` and `lvremove` (DECISIONS I-68). At start no
// snapshot is in flight, so every `snap-*` volume is an orphan: the api
// re-sends the interrupted Snapshot command and the re-run makes its own.
func (m *Manager) sweepStaleSnapshots(ctx context.Context) {
	vols, err := m.d.LVM.ListVolumes(ctx)
	if err != nil {
		m.d.Log.Warn("could not list volumes for the snapshot sweep", "event", "reconcile_snapshots", "err", err.Error())
		return
	}
	for _, v := range vols {
		if !strings.HasPrefix(v, "snap-") {
			continue
		}
		if err := m.d.LVM.RemoveVolume(ctx, v); err != nil {
			m.d.Log.Warn("stale snapshot volume not removed", "event", "reconcile_snapshots", "volume", v, "err", err.Error())
			continue
		}
		m.d.Log.Warn("removed stale snapshot volume", "event", "reconcile_snapshots", "volume", v)
	}
}

func (m *Manager) reconcileGuest(ctx context.Context, g *state.Guest, unitActive bool) {
	switch g.State {
	case StateRunning, StateStarting, StateCreating, StateStopping:
		if unitActive {
			if g.State != StateRunning {
				_ = m.setState(g, StateRunning, "reconciled: hypervisor running") // logged inside; nothing else to do on failure
			}
			m.startMonitor(g)
			return
		}
		m.teardown(ctx, g)
		if vol, _ := m.d.LVM.VolumeExists(ctx, VolumeName(g.GuestID)); !vol {
			_ = m.setState(g, StateError, "reconciled: volume missing") // see above
			return
		}
		_ = m.setState(g, StateStopped, "reconciled: hypervisor not running after hostd restart") // see above
	case StateRestoring:
		_ = m.setState(g, StateError, "restore interrupted by hostd restart; api must retry Restore") // see above
	case StateDestroying:
		_ = m.setState(g, StateError, "destroy interrupted by hostd restart; api must resend DestroyGuest") // see above
	case StateStopped, StateError:
		if unitActive {
			// The hypervisor outlived a state write; it is running, say so.
			_ = m.setState(g, StateRunning, "reconciled: hypervisor running") // see above
			m.startMonitor(g)
			return
		}
		if ok, _ := m.d.Net.TapExists(ctx, g.Tap); ok {
			m.teardown(ctx, g)
		}
		if v, _ := m.d.Systemd.IsActive(ctx, virtiofs.Unit(g.GuestID)); v {
			_ = virtiofs.Stop(ctx, m.d.Systemd, g.GuestID) // a stray virtiofsd; nothing depends on it
		}
	}
}

func (m *Manager) guestFromDisk(id string) *state.Guest {
	b, err := os.ReadFile(filepath.Join(m.guestDir(id), "guest.json"))
	if err != nil {
		return nil
	}
	g := &state.Guest{}
	if err := json.Unmarshal(b, g); err != nil || g.GuestID != id {
		return nil
	}
	return g
}

// Rebuild reconstructs the guest table from disk (guest.json in every
// guest directory, volumes, units) for `hostd reconcile` after a lost
// state.db. Returns the ids restored.
func (m *Manager) Rebuild(ctx context.Context) ([]string, error) {
	des, err := os.ReadDir(m.cfg.GuestsDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var ids []string
	for _, d := range des {
		if !d.IsDir() {
			continue
		}
		g := m.guestFromDisk(d.Name())
		if g == nil {
			continue
		}
		if _, err := m.d.State.GetGuest(g.GuestID); err == nil {
			continue
		}
		if err := m.d.State.ClaimIndex(g.GuestID, g.IPIndex); err != nil {
			m.d.Log.Warn("rebuild: address in use", "event", "reconcile_rebuild", "guest_id", g.GuestID, "err", err.Error())
			continue
		}
		if err := m.d.State.PutGuest(g); err != nil {
			return ids, err
		}
		ids = append(ids, g.GuestID)
	}
	return ids, m.Reconcile(ctx)
}
