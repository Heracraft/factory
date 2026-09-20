package billing

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return v.UTC()
}

// The billing period is the user's cycle anchored at signup (§5.1), not the
// calendar month.
func TestPeriodForAnchoredAtSignup(t *testing.T) {
	anchor := mustTime(t, "2026-09-17T14:23:00Z")
	for _, c := range []struct{ at, start, end string }{
		{"2026-09-17T14:23:00Z", "2026-09-17T14:23:00Z", "2026-10-17T14:23:00Z"}, // the anchor itself
		{"2026-09-30T23:00:00Z", "2026-09-17T14:23:00Z", "2026-10-17T14:23:00Z"}, // crosses the calendar month
		{"2026-10-01T00:00:00Z", "2026-09-17T14:23:00Z", "2026-10-17T14:23:00Z"},
		{"2026-10-17T14:23:00Z", "2026-10-17T14:23:00Z", "2026-11-17T14:23:00Z"}, // half-open at the boundary
		{"2027-03-02T00:00:00Z", "2027-02-17T14:23:00Z", "2027-03-17T14:23:00Z"}, // months later, no drift
	} {
		p := PeriodFor(anchor, mustTime(t, c.at))
		if !p.Start.Equal(mustTime(t, c.start)) || !p.End.Equal(mustTime(t, c.end)) {
			t.Errorf("at %s: got [%s, %s), want [%s, %s)", c.at, p.Start.Format(time.RFC3339), p.End.Format(time.RFC3339), c.start, c.end)
		}
		if !p.Contains(mustTime(t, c.at)) {
			t.Errorf("at %s is not in its own period", c.at)
		}
	}
}

// Stripe's anchor rule for short months: the 31st bills on the last day of
// a shorter month and comes back to the 31st afterwards. Go's AddDate turns
// 31 January plus a month into 3 March, which would bill two days of
// February twice.
func TestPeriodForClampsShortMonths(t *testing.T) {
	anchor := mustTime(t, "2026-01-31T00:00:00Z")
	for _, c := range []struct{ at, start, end string }{
		{"2026-02-01T00:00:00Z", "2026-01-31T00:00:00Z", "2026-02-28T00:00:00Z"},
		{"2026-03-01T00:00:00Z", "2026-02-28T00:00:00Z", "2026-03-31T00:00:00Z"},
		{"2026-04-01T00:00:00Z", "2026-03-31T00:00:00Z", "2026-04-30T00:00:00Z"},
		{"2026-05-30T00:00:00Z", "2026-04-30T00:00:00Z", "2026-05-31T00:00:00Z"},
	} {
		p := PeriodFor(anchor, mustTime(t, c.at))
		if !p.Start.Equal(mustTime(t, c.start)) || !p.End.Equal(mustTime(t, c.end)) {
			t.Errorf("at %s: got [%s, %s), want [%s, %s)", c.at, p.Start.Format(time.RFC3339), p.End.Format(time.RFC3339), c.start, c.end)
		}
	}
	// Periods tile the timeline: no hour is in two of them and none is in
	// none of them.
	prev := PeriodFor(anchor, anchor)
	for i := 0; i < 36; i++ {
		next := PeriodFor(anchor, prev.End)
		if !next.Start.Equal(prev.End) {
			t.Fatalf("period %d starts at %s, previous ended at %s", i, next.Start, prev.End)
		}
		prev = next
	}
}

// A zero anchor (an account from before the column existed, until the
// migration backfills it) falls back to the calendar month.
func TestPeriodForZeroAnchorIsCalendarMonth(t *testing.T) {
	p := PeriodFor(time.Time{}, mustTime(t, "2026-09-17T14:00:00Z"))
	if !p.Start.Equal(mustTime(t, "2026-09-01T00:00:00Z")) || !p.End.Equal(mustTime(t, "2026-10-01T00:00:00Z")) {
		t.Fatalf("got [%s, %s)", p.Start, p.End)
	}
	if p.Hours() != 720 {
		t.Fatalf("september is %d hours", p.Hours())
	}
}

func TestPeriodHours(t *testing.T) {
	anchor := mustTime(t, "2026-01-15T00:00:00Z")
	// January 15 to February 15 is 31 days; February 15 to March 15 is 28.
	if h := PeriodFor(anchor, anchor).Hours(); h != 31*24 {
		t.Errorf("jan period %d hours", h)
	}
	if h := PeriodFor(anchor, mustTime(t, "2026-02-20T00:00:00Z")).Hours(); h != 28*24 {
		t.Errorf("feb period %d hours", h)
	}
}
