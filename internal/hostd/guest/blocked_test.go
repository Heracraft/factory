package guest

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	hnet "github.com/heracraft/repose/internal/hostd/net"
)

// A guest goes over a threshold only when its blocked packets within the
// last BlockedWindow pass it, is reported once per crossing, and a guest
// that stops trying falls back under once its attempts age out of the
// window (DECISIONS I-238..I-240).
func TestBlockedTrackerWindowAndCrossings(t *testing.T) {
	b := newBlockedTracker()
	t0 := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	at := func(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }
	c := func(smtp, stratum, flows uint64) map[string]map[string]uint64 {
		return map[string]map[string]uint64{
			"g1": {hnet.BlockedSMTP: smtp, hnet.BlockedStratum: stratum, hnet.BlockedFlows: flows},
			"g2": {hnet.BlockedSMTP: 7}, // a guest that tried port 25 once, long ago
		}
	}
	d, over, crossed := b.observe(at(0), c(500, 0, 0))
	if d[hnet.BlockedSMTP] != 0 || over[hnet.BlockedSMTP] != 0 || len(crossed) != 0 {
		t.Fatalf("first reading is a baseline: deltas %v over %v crossed %v", d, over, crossed)
	}
	// 60 packets a minute at port 25: over 100 within the window after two minutes.
	d, over, crossed = b.observe(at(1), c(560, 0, 0))
	if d[hnet.BlockedSMTP] != 60 || over[hnet.BlockedSMTP] != 0 || len(crossed) != 0 {
		t.Fatalf("minute 1: deltas %v over %v crossed %v", d, over, crossed)
	}
	_, over, crossed = b.observe(at(2), c(620, 0, 0))
	if over[hnet.BlockedSMTP] != 1 || len(crossed) != 1 || crossed[0] != (blockedCrossing{guestID: "g1", kind: hnet.BlockedSMTP, packets: 120}) {
		t.Fatalf("minute 2: over %v crossed %v", over, crossed)
	}
	// Still going: over, but not a new crossing.
	_, over, crossed = b.observe(at(3), c(680, 0, 0))
	if over[hnet.BlockedSMTP] != 1 || len(crossed) != 0 {
		t.Fatalf("minute 3: over %v crossed %v", over, crossed)
	}
	// It stops at minute 3; at minute 13 the window holds nothing new.
	for m := 4; m <= 12; m++ {
		b.observe(at(m), c(680, 0, 0))
	}
	_, over, _ = b.observe(at(13), c(680, 0, 0))
	if over[hnet.BlockedSMTP] != 0 {
		t.Fatalf("attempts older than the window still count: %v", over)
	}
	// A miner retrying its pool: 31 packets is over the stratum threshold;
	// flows needs more than 1000.
	_, over, crossed = b.observe(at(14), c(680, 31, 1000))
	if over[hnet.BlockedStratum] != 1 || over[hnet.BlockedFlows] != 0 || len(crossed) != 1 || crossed[0].kind != hnet.BlockedStratum {
		t.Fatalf("stratum/flows: over %v crossed %v", over, crossed)
	}
	// A destroyed guest is forgotten; one whose counter was recreated
	// (smaller than before) starts a new baseline, never a negative delta.
	d, _, _ = b.observe(at(15), map[string]map[string]uint64{"g1": {hnet.BlockedSMTP: 3}})
	if _, ok := b.readings["g2"]; ok {
		t.Fatal("g2 is gone from the counters but still tracked")
	}
	if d[hnet.BlockedSMTP] != 0 {
		t.Fatalf("a recreated counter gave delta %d", d[hnet.BlockedSMTP])
	}
}

// CollectSamples reads the blocked counters and exports them per reason,
// without any guest id in a label.
func TestSamplesExportBlockedCounters(t *testing.T) {
	h := newHarness(t, nil)
	h.create(gid1)
	ctx := context.Background()
	h.net.Block(gid1, hnet.BlockedSMTP, 10)
	h.m.CollectSamples(ctx)
	h.net.Block(gid1, hnet.BlockedSMTP, 200)
	h.net.Block(gid1, hnet.BlockedFlows, 5)
	h.m.CollectSamples(ctx)
	if got := testutil.ToFloat64(h.metrics.EgressBlockedTotal.WithLabelValues("smtp")); got != 200 {
		t.Fatalf("repose_host_egress_blocked_total{reason=smtp} = %v, want 200 (the first reading is the baseline)", got)
	}
	if got := testutil.ToFloat64(h.metrics.EgressBlockedTotal.WithLabelValues("flows")); got != 5 {
		t.Fatalf("flows = %v", got)
	}
	if got := testutil.ToFloat64(h.metrics.EgressBlockedGuests.WithLabelValues("smtp")); got != 1 {
		t.Fatalf("repose_host_egress_blocked_guests{reason=smtp} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(h.metrics.EgressBlockedGuests.WithLabelValues("stratum")); got != 0 {
		t.Fatalf("stratum guests = %v", got)
	}
}
