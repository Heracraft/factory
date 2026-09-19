package guest

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
	"github.com/heracraft/repose/internal/hostd/virtiofs"
)

// levelNotice sits between Info and Warn, for audit lines.
const levelNotice = slog.Level(2)

type sampleCursor struct {
	cpu, rx, tx, egress uint64
	seen                bool
}

func (m *Manager) hostSample() *hostdv1.HostSample {
	hs := &hostdv1.HostSample{PoolFree: m.PoolFreeBytes()}
	if m.d.MemInfo != nil {
		if _, avail, err := m.d.MemInfo(); err == nil {
			hs.MemFree = avail
		}
	}
	if m.d.Load1 != nil {
		hs.Load1 = m.d.Load1()
	}
	m.mu.Lock()
	hs.BuildsRunning = uint32(m.buildRun)
	m.mu.Unlock()
	return hs
}

// CollectSamples builds one Samples message for every guest on the host.
func (m *Manager) CollectSamples(ctx context.Context) *hostdv1.Samples {
	s := &hostdv1.Samples{Ts: m.d.Now().Unix(), Host: m.hostSample()}
	gs, err := m.d.State.ListGuests()
	if err != nil {
		m.d.Log.Error("samples: state read failed", "component", "hostd", "event", "samples", "err", err.Error())
		return s
	}
	lost := 0
	for _, g := range gs {
		gsm := &hostdv1.GuestSample{GuestId: g.GuestID, State: g.State, Class: g.Class, DiskAllocBytes: g.VolumeBytes, Signals: &hostdv1.GuestSignals{}}
		if size, used, err := m.d.LVM.VolumeStats(ctx, VolumeName(g.GuestID)); err == nil {
			gsm.DiskAllocBytes, gsm.DiskUsedBytes = size, used
		}
		if g.State == StateRunning {
			m.mu.Lock()
			cur := m.last[g.GuestID]
			m.mu.Unlock()
			next := cur
			if props, err := m.d.Systemd.Show(ctx, GuestUnit(g.GuestID), "CPUUsageNSec", "MemoryCurrent"); err == nil {
				cpu, _ := strconv.ParseUint(props["CPUUsageNSec"], 10, 64)
				mem, _ := strconv.ParseUint(props["MemoryCurrent"], 10, 64)
				gsm.MemRssBytes = mem
				if cur.seen && cpu >= cur.cpu {
					gsm.CpuNsDelta = cpu - cur.cpu
				}
				next.cpu = cpu
			}
			if rx, tx, err := m.d.Net.TapStats(g.Tap); err == nil {
				if cur.seen && rx >= cur.rx {
					gsm.NetRxBytesDelta = rx - cur.rx
				}
				if cur.seen && tx >= cur.tx {
					gsm.NetTxBytesDelta = tx - cur.tx
				}
				next.rx, next.tx = rx, tx
			}
			if eg, err := m.d.Net.CounterBytes(ctx, g.GuestID); err == nil {
				next.egress = eg
			}
			next.seen = true
			m.mu.Lock()
			m.last[g.GuestID] = next
			m.mu.Unlock()
			if sess, err := m.session(g.GuestID); err == nil {
				sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				sr, err := sess.Sample(sctx)
				cancel()
				if err == nil && sr != nil {
					if sr.Signals != nil {
						gsm.Signals = sr.Signals
					}
					gsm.Signals.GuestdOk = true
					gsm.Procs = sr.Procs
				} else {
					lost++
				}
			} else {
				lost++
			}
			if m.d.Metrics != nil {
				m.d.Metrics.GuestCPUSecondsTotal.WithLabelValues(g.Class).Add(float64(gsm.CpuNsDelta) / 1e9)
				m.d.Metrics.GuestNetBytesTotal.WithLabelValues("rx").Add(float64(gsm.NetRxBytesDelta))
				m.d.Metrics.GuestNetBytesTotal.WithLabelValues("tx").Add(float64(gsm.NetTxBytesDelta))
			}
		}
		s.Guests = append(s.Guests, gsm)
	}
	if m.d.Metrics != nil {
		m.d.Metrics.GuestdLost.Set(float64(lost))
		m.d.Metrics.PoolFreeBytes.Set(float64(s.Host.PoolFree))
		if size, _, err := m.d.LVM.PoolStats(ctx); err == nil {
			m.d.Metrics.PoolBytes.Set(float64(size))
		}
		m.d.Metrics.MemFreeBytes.Set(float64(m.FreeMemBytes()))
		m.d.Metrics.MemReservedBytes.Set(float64(m.reservedBytes()))
	}
	m.poolWarning()
	return s
}

var poolWarned time.Time

func (m *Manager) poolWarning() {
	pct, err := m.poolUsedPct()
	if err != nil || pct < m.cfg.PoolWarnPct {
		return
	}
	if m.d.Now().Sub(poolWarned) < 10*time.Minute {
		return
	}
	poolWarned = m.d.Now()
	m.d.Log.Warn("thin pool high", "component", "hostd", "event", "pool_warning", "pct", int(pct))
	m.Warn("pool_high", strconv.Itoa(int(pct))+"% of the thin pool is used")
}

// SnapshotAll enqueues a scheduled Snapshot for every running guest
// without the api (the nightly timer). Returns the guest ids queued.
func (m *Manager) SnapshotAll(reason string) ([]string, error) {
	gs, err := m.d.State.ListGuests()
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, g := range gs {
		if g.State != StateRunning {
			continue
		}
		id := "local-" + reason + "-" + g.GuestID + "-" + strconv.FormatInt(m.d.Now().Unix(), 10)
		m.Dispatch(&hostdv1.Command{CommandId: id, Cmd: &hostdv1.Command_Snapshot{Snapshot: &hostdv1.Snapshot{GuestId: g.GuestID, Reason: reason}}})
		ids = append(ids, g.GuestID)
	}
	return ids, nil
}

// Status is the operator view of a guest.
type Status struct {
	GuestID   string `json:"guest_id"`
	ProjectID string `json:"project_id"`
	Class     string `json:"class"`
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"`
	IP        string `json:"ip"`
	CID       uint32 `json:"vsock_cid"`
	Unit      string `json:"unit"`
	Virtiofsd string `json:"virtiofsd"`
	GuestdOK  bool   `json:"guestd_ok"`
	Closure   string `json:"system_closure"`
}

// Guests lists every guest with its live unit states.
func (m *Manager) Guests(ctx context.Context) ([]Status, error) {
	gs, err := m.d.State.ListGuests()
	if err != nil {
		return nil, err
	}
	var out []Status
	for _, g := range gs {
		st := Status{GuestID: g.GuestID, ProjectID: g.ProjectID, Class: g.Class, State: g.State, Reason: g.Reason, IP: g.IP, CID: g.CID, Closure: g.SystemClosure, Unit: "inactive", Virtiofsd: "inactive"}
		if a, _ := m.d.Systemd.IsActive(ctx, GuestUnit(g.GuestID)); a {
			st.Unit = "active"
		}
		if a, _ := m.d.Systemd.IsActive(ctx, virtiofs.Unit(g.GuestID)); a {
			st.Virtiofsd = "active"
		}
		if mon := m.monitorOf(g.GuestID); mon != nil && mon.current() != nil {
			st.GuestdOK = true
		}
		out = append(out, st)
	}
	return out, nil
}
