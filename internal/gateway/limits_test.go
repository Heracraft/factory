package gateway

import (
	"testing"
	"time"
)

func TestLimiterConcurrentAuthPerSource(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := newLimiter(4, func() time.Time { return now })
	for i := 0; i < 4; i++ {
		if !l.beginAuth("198.51.100.7") {
			t.Fatalf("attempt %d refused", i)
		}
	}
	if l.beginAuth("198.51.100.7") {
		t.Fatal("fifth concurrent attempt allowed")
	}
	if !l.beginAuth("198.51.100.8") {
		t.Fatal("another source refused")
	}
	l.endAuth("198.51.100.7", false)
	if !l.beginAuth("198.51.100.7") {
		t.Fatal("slot not released")
	}
}

func TestLimiterBansAfterTwentyFailures(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := newLimiter(4, func() time.Time { return now })
	for i := 0; i < 19; i++ {
		if !l.beginAuth("src") {
			t.Fatalf("refused at failure %d", i)
		}
		l.endAuth("src", true)
	}
	if !l.beginAuth("src") {
		t.Fatal("banned before the 20th failure")
	}
	l.endAuth("src", true)
	if l.beginAuth("src") {
		t.Fatal("not banned after 20 failures")
	}
	now = now.Add(banDuration + time.Second)
	if !l.beginAuth("src") {
		t.Fatal("ban did not expire")
	}
	l.endAuth("src", false)
	// A success clears the failure history.
	for i := 0; i < 19; i++ {
		l.beginAuth("src")
		l.endAuth("src", true)
	}
	l.beginAuth("src")
	l.endAuth("src", false)
	l.beginAuth("src")
	l.endAuth("src", true)
	if !l.beginAuth("src") {
		t.Fatal("failures before a success counted towards the ban")
	}
}

func TestConnCounter(t *testing.T) {
	c := &connCounter{max: 2}
	if !c.acquire() || !c.acquire() {
		t.Fatal("first two refused")
	}
	if c.acquire() {
		t.Fatal("third accepted over the cap")
	}
	c.release()
	if !c.acquire() {
		t.Fatal("release did not free a slot")
	}
	if c.open() != 2 {
		t.Fatalf("open %d", c.open())
	}
}
