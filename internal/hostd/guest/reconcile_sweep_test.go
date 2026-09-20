package guest

import (
	"context"
	"testing"

	"github.com/heracraft/repose/internal/hostd/lvm"
)

// A hostd that starts finds no snapshot in flight, so every snap-* volume
// is an orphan of an interrupted Snapshot (DECISIONS I-68).
func TestReconcileRemovesStaleSnapshotVolumes(t *testing.T) {
	h := newHarness(t, nil)
	h.lvm.Volumes["snap-g1-1"] = &lvm.FakeVolume{}
	h.lvm.Volumes["g-g1"] = &lvm.FakeVolume{}
	if err := h.m.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.lvm.Volumes["snap-g1-1"]; ok {
		t.Fatal("stale snapshot volume survived reconcile")
	}
	if _, ok := h.lvm.Volumes["g-g1"]; !ok {
		t.Fatal("guest volume was removed by the sweep")
	}
}
