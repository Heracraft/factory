package httpapi

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestParseWaitCaps(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"": 0, "0": 0, "5s": 5 * time.Second, "1500ms": 1500 * time.Millisecond,
		"7": 7 * time.Second, "25s": OpWaitMax, "600": OpWaitMax, "1h": OpWaitMax,
	} {
		got, err := parseWait(in)
		if err != nil || got != want {
			t.Errorf("parseWait(%q) = %s, %v; want %s", in, got, err, want)
		}
	}
	for _, in := range []string{"soon", "-1s", "-3", "1.5"} {
		if _, err := parseWait(in); err == nil {
			t.Errorf("parseWait(%q) accepted", in)
		}
	}
}

func TestOpWaitersBound(t *testing.T) {
	var w opWaiters
	a, b := uuid.New(), uuid.New()
	var rel []func()
	for i := 0; i < OpWaitersPerUser; i++ {
		r, ok := w.acquire(a)
		if !ok {
			t.Fatalf("slot %d refused", i)
		}
		rel = append(rel, r)
	}
	if _, ok := w.acquire(a); ok {
		t.Fatal("over the per-user bound")
	}
	rb, ok := w.acquire(b)
	if !ok {
		t.Fatal("another user refused")
	}
	rb()
	rel[0]()
	if r, ok := w.acquire(a); !ok {
		t.Fatal("released slot not reusable")
	} else {
		r()
	}
	for _, r := range rel[1:] {
		r()
	}
	if w.total != 0 || len(w.per) != 0 {
		t.Fatalf("leak: total %d per %v", w.total, w.per)
	}
	w.total = opWaitersTotal
	if _, ok := w.acquire(b); ok {
		t.Fatal("over the total bound")
	}
}
