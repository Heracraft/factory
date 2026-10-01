package guest

import (
	"strings"
	"testing"
	"time"

	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
)

func noSnapLeft(t *testing.T, h *harness) {
	t.Helper()
	for name := range h.lvm.Volumes {
		if strings.HasPrefix(name, "snap-") {
			t.Fatalf("lvm snapshot %s left behind", name)
		}
	}
}

// TestStopUploadsWhileTheGuestShutsDown: a stop with snapshot_first
// uploads the snapshot it froze while the guest powers off (I-370). The
// upload here waits until the guest has left running; with the old order
// (upload, then stop) it would wait out its timeout.
func TestStopUploadsWhileTheGuestShutsDown(t *testing.T) {
	h := newHarness(t, nil)
	h.create(gid1)
	h.lvm.SetData("g-"+gid1, []byte("the tenant's filesystem"))
	overlapped := make(chan bool, 1)
	h.blob.BeforeUpload = func(string) {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			st := ""
			if g, err := h.st.GetGuest(gid1); err == nil && g != nil {
				st = g.State
			}
			if st == StateStopping || st == StateStopped {
				overlapped <- true
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		overlapped <- false
	}
	gd := h.guestd(gid1)
	res := h.mustOK(cmd(&hostdv1.StopGuest{GuestId: gid1, SnapshotFirst: true}))
	if !<-overlapped {
		t.Fatal("the upload did not run while the guest was stopping")
	}
	sr := res.GetStop()
	if sr.BlobPath == "" || sr.Bytes == 0 || string(h.blob.Blobs[sr.BlobPath]) != "the tenant's filesystem" {
		t.Fatalf("stop result %v", sr)
	}
	if h.blob.Meta[sr.BlobPath]["reason"] != "stop" {
		t.Fatalf("metadata %v", h.blob.Meta[sr.BlobPath])
	}
	if h.guest(gid1).State != StateStopped {
		t.Fatalf("state %s", h.guest(gid1).State)
	}
	if kinds := strings.Join(gd.Kinds(), ","); !strings.Contains(kinds, "Freeze,Thaw,Shutdown") {
		t.Fatalf("freeze/thaw missing: %s", kinds)
	}
	noSnapLeft(t, h)
}

// TestStopRetriesTheSnapshotFromTheStoppedVolume: an upload that fails
// while the guest stops is taken again from the stopped volume; the stop
// succeeds with that snapshot.
func TestStopRetriesTheSnapshotFromTheStoppedVolume(t *testing.T) {
	h := newHarness(t, nil)
	h.create(gid1)
	h.lvm.SetData("g-"+gid1, []byte("data"))
	h.blob.FailNext = 1
	res := h.mustOK(cmd(&hostdv1.StopGuest{GuestId: gid1, SnapshotFirst: true}))
	if sr := res.GetStop(); sr.BlobPath == "" || string(h.blob.Blobs[sr.BlobPath]) != "data" {
		t.Fatalf("stop result %v", sr)
	}
	if len(h.blob.Blobs) != 1 {
		t.Fatalf("%d blobs, want the retry's one", len(h.blob.Blobs))
	}
	if h.guest(gid1).State != StateStopped {
		t.Fatalf("state %s", h.guest(gid1).State)
	}
	noSnapLeft(t, h)
}

// TestStopReportsASnapshotThatFailsTwice: when the retry fails too, the
// stop reports the upload failure; the guest is down and its volume kept.
func TestStopReportsASnapshotThatFailsTwice(t *testing.T) {
	h := newHarness(t, nil)
	h.create(gid1)
	h.blob.FailNext = 2
	res := h.mustFail(cmd(&hostdv1.StopGuest{GuestId: gid1, SnapshotFirst: true}), CodeInternal)
	if !strings.Contains(res.Error.Message, "snapshot upload failed") {
		t.Fatalf("message %s", res.Error.Message)
	}
	if h.guest(gid1).State != StateStopped {
		t.Fatalf("state %s", h.guest(gid1).State)
	}
	if _, ok := h.lvm.Volumes["g-"+gid1]; !ok {
		t.Fatal("volume gone")
	}
	noSnapLeft(t, h)
	// The api's next start boots it as usual.
	h.mustOK(cmd(&hostdv1.StartGuest{GuestId: gid1}))
}

// TestStopWithAFailedFreezeStopsNothing: a freeze guestd refuses fails
// the stop before anything shuts down, as before I-370; the api's
// recovery (I-157) then stops without a snapshot and snapshots after.
func TestStopWithAFailedFreezeStopsNothing(t *testing.T) {
	h := newHarness(t, nil)
	h.gopts.Fail = map[string]*guestdv1.Error{"Freeze": {Code: "internal", Message: "fsfreeze: busy"}}
	h.create(gid1)
	h.mustFail(cmd(&hostdv1.StopGuest{GuestId: gid1, SnapshotFirst: true}), CodeGuestUnresponsive)
	if h.guest(gid1).State != StateRunning {
		t.Fatalf("state %s, want running", h.guest(gid1).State)
	}
	if len(h.blob.Blobs) != 0 {
		t.Fatal("uploaded without a freeze")
	}
	noSnapLeft(t, h)
}
