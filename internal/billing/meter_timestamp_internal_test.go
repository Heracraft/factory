package billing

import (
	"testing"
	"time"
)

// DECISIONS I-179: the rollup's period starts at the anchor's hour, Stripe's
// at the exact anchor second. With every row reported at the last second of
// its hour, an hour belongs to the same period on both sides, over short
// months and the 31st-anchored clamp included.
func TestMeterTimestampLandsInTheRollupsPeriod(t *testing.T) {
	for _, anchor := range []time.Time{
		time.Date(2026, 10, 3, 14, 32, 10, 0, time.UTC),
		time.Date(2027, 1, 31, 23, 59, 59, 0, time.UTC),
		time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 30, 0, 0, 1, 0, time.UTC),
	} {
		ours := anchor.Truncate(time.Hour)
		for h := ours.Add(-48 * time.Hour); h.Before(ours.AddDate(0, 4, 0)); h = h.Add(time.Hour) {
			rollup := PeriodFor(ours, h)
			ts := time.Unix(meterTimestamp(h), 0).UTC()
			stripe := PeriodFor(anchor, ts)
			if !stripe.Start.Truncate(time.Hour).Equal(rollup.Start) {
				t.Fatalf("anchor %s, hour %s: rollup period starts %s, Stripe's (event at %s) %s",
					anchor, h, rollup.Start, ts, stripe.Start)
			}
		}
	}
}
