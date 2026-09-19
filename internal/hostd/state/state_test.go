package state

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func open(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestSecondOpenIsLocked(t *testing.T) {
	d := open(t)
	_, err := Open(d.Path())
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked, got %v", err)
	}
}

func TestGuestsAndIndexes(t *testing.T) {
	d := open(t)
	if err := d.PutGuest(&Guest{GuestID: "g1", State: "creating"}); err != nil {
		t.Fatal(err)
	}
	i1, err := d.AllocIndex("g1", 1020)
	if err != nil || i1 != 0 {
		t.Fatalf("first index: %d %v", i1, err)
	}
	i2, _ := d.AllocIndex("g2", 1020)
	if i2 != 1 {
		t.Fatalf("second index: %d", i2)
	}
	again, _ := d.AllocIndex("g1", 1020)
	if again != 0 {
		t.Fatalf("realloc must return the held index, got %d", again)
	}
	if err := d.ReleaseIndex("g1"); err != nil {
		t.Fatal(err)
	}
	i3, _ := d.AllocIndex("g3", 1020)
	if i3 != 0 {
		t.Fatalf("released index must be reused, got %d", i3)
	}
	if _, err := d.AllocIndex("g4", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AllocIndex("g5", 2); err == nil {
		t.Fatal("range exhausted must fail")
	}
	g, err := d.SetGuestState("g1", "running", "")
	if err != nil || g.State != "running" {
		t.Fatalf("set state: %v %v", g, err)
	}
	if _, err := d.GetGuest("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestCommandsAndExport(t *testing.T) {
	d := open(t)
	ex, err := d.StartCommand(&Command{CommandID: "c1", Kind: "CreateGuest", GuestID: "g1"})
	if err != nil || ex != nil {
		t.Fatalf("start: %v %v", ex, err)
	}
	ex, _ = d.StartCommand(&Command{CommandID: "c1"})
	if ex == nil || ex.Status != "started" {
		t.Fatalf("second start must return the existing started record, got %v", ex)
	}
	if err := d.FinishCommand("c1", []byte("r")); err != nil {
		t.Fatal(err)
	}
	ex, _ = d.StartCommand(&Command{CommandID: "c1"})
	if ex.Status != "done" || string(ex.Result) != "r" {
		t.Fatalf("expected done record, got %v", ex)
	}
	n, err := d.PruneCommands(time.Now().Add(time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("prune: %d %v", n, err)
	}
	_ = d.PutGuest(&Guest{GuestID: "g1", IPIndex: 3})
	_ = d.SetDraining(true)
	var buf bytes.Buffer
	if err := d.Export(&buf); err != nil {
		t.Fatal(err)
	}
	d2 := open(t)
	if err := d2.Import(&buf); err != nil {
		t.Fatal(err)
	}
	gs, _ := d2.ListGuests()
	dr, _ := d2.Draining()
	if len(gs) != 1 || gs[0].GuestID != "g1" || !dr {
		t.Fatalf("import mismatch: %v %v", gs, dr)
	}
	if i, _ := d2.AllocIndex("g1", 10); i != 3 {
		t.Fatalf("imported index not restored: %d", i)
	}
}
