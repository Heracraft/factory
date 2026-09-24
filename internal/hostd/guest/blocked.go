package guest

import (
	"context"
	"sort"
	"time"

	hnet "github.com/heracraft/repose/internal/hostd/net"
)

// BlockedWindow is how far back a guest's blocked attempts are summed to
// decide whether it hits a block persistently (DECISIONS I-238..I-240).
const BlockedWindow = 10 * time.Minute

// BlockedThresholds are the packets dropped within BlockedWindow above
// which a guest counts toward repose_host_egress_blocked_guests, the
// input of the EgressBlocked alert. A blocked TCP connect is about six
// SYNs (the kernel's retransmits), so 100 is some fifteen attempts at port
// 25 in ten minutes: an app that tried to mail once and gave up stays
// under it, a spam loop does not. Any miner retries its pool every few
// seconds, so 30 packets is already five attempts. A flow over the rate is
// one packet, and a scan drops thousands a minute.
var BlockedThresholds = map[string]uint64{
	hnet.BlockedSMTP:    100,
	hnet.BlockedStratum: 30,
	hnet.BlockedFlows:   1000,
}

type blockedReading struct {
	at    time.Time
	value uint64
}

// blockedTracker keeps each guest's recent readings of its blocked
// counters. hostd restarting loses the history, which only delays an
// alert by one window.
type blockedTracker struct {
	readings map[string]map[string][]blockedReading // guest -> kind -> oldest first
	over     map[string]map[string]bool             // guest -> kind -> over the threshold at the last reading
}

func newBlockedTracker() *blockedTracker {
	return &blockedTracker{readings: map[string]map[string][]blockedReading{}, over: map[string]map[string]bool{}}
}

// blockedCrossing is a guest going over a kind's threshold.
type blockedCrossing struct {
	guestID string
	kind    string
	packets uint64 // within BlockedWindow
}

// observe records one reading of every guest's counters and returns the
// packets dropped since the previous reading per kind (for the counter
// metric; a guest's first reading is its baseline), the number of guests
// over each kind's threshold now, and the guests that went over since the
// last reading. Guests absent from counts (destroyed) are forgotten.
func (b *blockedTracker) observe(now time.Time, counts map[string]map[string]uint64) (deltas map[string]uint64, over map[string]int, crossed []blockedCrossing) {
	deltas, over = map[string]uint64{}, map[string]int{}
	for _, k := range hnet.BlockedKinds {
		deltas[k], over[k] = 0, 0
	}
	for g := range b.readings {
		if _, ok := counts[g]; !ok {
			delete(b.readings, g)
			delete(b.over, g)
		}
	}
	guests := make([]string, 0, len(counts))
	for g := range counts {
		guests = append(guests, g)
	}
	sort.Strings(guests)
	for _, g := range guests {
		if b.readings[g] == nil {
			b.readings[g] = map[string][]blockedReading{}
			b.over[g] = map[string]bool{}
		}
		for _, k := range hnet.BlockedKinds {
			v := counts[g][k]
			rs := b.readings[g][k]
			if n := len(rs); n > 0 && v >= rs[n-1].value {
				deltas[k] += v - rs[n-1].value
			} else if n > 0 {
				rs = nil // the counter was recreated: start again
			}
			rs = append(rs, blockedReading{at: now, value: v})
			// Keep the newest reading at or before the window's start, so
			// the difference covers the whole window.
			for len(rs) > 1 && !rs[1].at.After(now.Add(-BlockedWindow)) {
				rs = rs[1:]
			}
			b.readings[g][k] = rs
			inWindow := v - rs[0].value
			isOver := inWindow > BlockedThresholds[k]
			if isOver {
				over[k]++
				if !b.over[g][k] {
					crossed = append(crossed, blockedCrossing{guestID: g, kind: k, packets: inWindow})
				}
			}
			b.over[g][k] = isOver
		}
	}
	return deltas, over, crossed
}

// sampleBlocked reads the blocked counters once and updates the metrics;
// a guest going over a threshold is logged with its id, which the
// EgressBlocked alert's runbook entry tells the operator to look up.
func (m *Manager) sampleBlocked(ctx context.Context) {
	counts, err := m.d.Net.BlockedPackets(ctx)
	if err != nil {
		m.d.Log.Warn("blocked counters unreadable", "event", "egress_blocked_read", "err", err.Error())
		return
	}
	m.mu.Lock()
	if m.blocked == nil {
		m.blocked = newBlockedTracker()
	}
	deltas, over, crossed := m.blocked.observe(m.d.Now(), counts)
	m.mu.Unlock()
	if m.d.Metrics != nil {
		for k, d := range deltas {
			m.d.Metrics.EgressBlockedTotal.WithLabelValues(k).Add(float64(d))
		}
		for k, n := range over {
			m.d.Metrics.EgressBlockedGuests.WithLabelValues(k).Set(float64(n))
		}
	}
	for _, c := range crossed {
		m.d.Log.Warn("guest keeps hitting an egress block", "event", "egress_blocked", "guest_id", c.guestID, "reason", c.kind, "count", c.packets, "window_ms", BlockedWindow.Milliseconds())
	}
}
