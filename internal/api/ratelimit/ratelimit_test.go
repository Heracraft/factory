package ratelimit

import (
	"testing"
	"time"
)

func TestBucket(t *testing.T) {
	l := New(5)
	now := time.Unix(1000, 0)
	l.now = func() time.Time { return now }
	for i := 0; i < 5; i++ {
		if ok, _ := l.Allow("u"); !ok {
			t.Fatalf("request %d refused", i)
		}
	}
	ok, retry := l.Allow("u")
	if ok || retry < time.Second || retry > 13*time.Second {
		t.Fatalf("ok=%v retry=%v", ok, retry)
	}
	if ok, _ := l.Allow("other"); !ok {
		t.Fatal("keys are independent")
	}
	now = now.Add(12 * time.Second)
	if ok, _ := l.Allow("u"); !ok {
		t.Fatal("a token should have refilled after 12 s at 5/min")
	}
	now = now.Add(time.Hour)
	l.Sweep(time.Minute)
	if len(l.buckets) != 0 {
		t.Fatal("sweep kept idle buckets")
	}
}
