package billing

import "testing"

// §9: "a guest running 720 hours in a period is charged exactly the cap;
// 360 hours exactly half". Run the hours through Price the way the rollup
// does, carrying the period running total.
func TestCapOverAWholePeriod(t *testing.T) {
	for _, c := range []struct {
		class       string
		hours       int
		periodHours int
		wantGuest   int64
	}{
		{"large", 720, 720, CapLarge}, // 720 * 14 = 10080 > 9900, so the cap bites near the end
		{"large", 360, 720, 360 * HourLarge},
		{"small", 720, 720, CapSmall},
		{"xl", 720, 720, CapXL},
		{"small", 360, 720, 360 * HourSmall},
	} {
		var total int64
		for i := 0; i < c.hours; i++ {
			r := Price(Inputs{Class: c.class, RunningSeconds: 3600, PeriodHours: c.periodHours, MonthGuestCents: total})
			total += r.GuestCents
		}
		if total != c.wantGuest {
			t.Errorf("%s for %d hours: %d cents, want %d", c.class, c.hours, total, c.wantGuest)
		}
		if total > Cap(c.class) {
			t.Errorf("%s for %d hours exceeded the cap: %d > %d", c.class, c.hours, total, Cap(c.class))
		}
	}
	// Half a period is exactly half the cap for small and xl, whose hourly
	// rate divides the cap evenly enough; for large, 360 * 14 = 5040, which
	// is what an hourly rate rounded up to the cent gives and is above half
	// the cap by design (PRICING.md: "hourly is the cap divided by 720
	// rounded up to the cent").
	if got := int64(360) * HourLarge; got != 5040 {
		t.Fatalf("360 large hours: %d", got)
	}
}

// §5.1: a project that changes class mid-period is capped at the sum of
// hours_in_class * hourly, bounded by the larger class's cap. The two
// failures the rule exists to prevent are a downgrade-then-upgrade evading
// the cap and a one-hour XL trial being overcharged.
func TestClassChangeMidPeriodCap(t *testing.T) {
	if LargerClass("small", "xl") != "xl" || LargerClass("xl", "large") != "xl" || LargerClass("", "small") != "small" || LargerClass("", "") != "" {
		t.Fatal("LargerClass does not order the classes")
	}
	// One hour of XL then 719 of small: the uncapped sum is 28 + 719*7 =
	// 5061, under the XL cap, so nothing is clipped and the one-hour XL
	// trial is not overcharged.
	var total int64
	capClass := ""
	for i := 0; i < 720; i++ {
		class := "small"
		if i == 0 {
			class = "xl"
		}
		capClass = LargerClass(capClass, class)
		r := Price(Inputs{Class: class, RunningSeconds: 3600, CapClass: capClass, PeriodHours: 720, MonthGuestCents: total})
		if r.CapApplied {
			t.Fatalf("hour %d was clipped: %+v", i, r)
		}
		total += r.GuestCents
	}
	if total != HourXL+719*HourSmall {
		t.Fatalf("xl then small: %d cents", total)
	}
	// 360 hours of large then 360 of XL: 360*14 + 360*28 = 15120, under the
	// XL cap of 19900, so again nothing is clipped; a full period of XL
	// still stops at the XL cap.
	total, capClass = 0, ""
	for i := 0; i < 720; i++ {
		class := "large"
		if i >= 360 {
			class = "xl"
		}
		capClass = LargerClass(capClass, class)
		r := Price(Inputs{Class: class, RunningSeconds: 3600, CapClass: capClass, PeriodHours: 720, MonthGuestCents: total})
		total += r.GuestCents
	}
	if total != 360*HourLarge+360*HourXL {
		t.Fatalf("large then xl: %d cents", total)
	}
	// The evasion the rule blocks: a period spent at XL and ending on small
	// is still held to the XL cap, so the bill is the hours actually used
	// (700*28 + 20*7 = 19740) and not clipped to the small cap of 4900 that
	// the last hour's class would have named.
	total, capClass = 0, ""
	for i := 0; i < 720; i++ {
		class := "xl"
		if i >= 700 {
			class = "small"
		}
		capClass = LargerClass(capClass, class)
		r := Price(Inputs{Class: class, RunningSeconds: 3600, CapClass: capClass, PeriodHours: 720, MonthGuestCents: total})
		if r.CapCents != CapXL {
			t.Fatalf("hour %d was held to the %d cent cap, want the xl cap", i, r.CapCents)
		}
		total += r.GuestCents
	}
	if want := int64(700*HourXL + 20*HourSmall); total != want {
		t.Fatalf("xl then small ending: %d cents, want %d", total, want)
	}
	if total <= CapSmall {
		t.Fatalf("the downgrade brought the period under the small cap: %d", total)
	}
}

// The storage line sums to exactly gb * 10 over a period of any length,
// which the remainder exists for (§5.4).
func TestStorageSumsExactlyOverAnyPeriod(t *testing.T) {
	for _, hours := range []int{672, 696, 720, 744} {
		for _, gb := range []int64{1, 20, 40, 80, 137} {
			var total, rem int64
			for i := 0; i < hours; i++ {
				r := Price(Inputs{Class: "small", GBAlloc: gb, PeriodHours: hours, StorageRemainder: rem})
				total += r.StorageCents
				rem = r.StorageRemainder
			}
			if want := gb * StoragePerGBMonth; total != want {
				t.Errorf("%d GB over %d hours: %d cents, want %d", gb, hours, total, want)
			}
		}
	}
}

// The threshold hour is charged only for the excess (§5.4).
func TestEgressThresholdHour(t *testing.T) {
	// Exactly at the allowance: nothing.
	if r := Price(Inputs{Class: "small", MonthEgressBytes: 0, EgressBytes: EgressIncludedGB << 30, PeriodHours: 720}); r.EgressCents != 0 {
		t.Fatalf("at the allowance: %+v", r)
	}
	// One byte over: the excess is priced per byte at the per-GB rate and
	// rounded up to the cent, so it is one cent rather than a whole GB.
	if r := Price(Inputs{Class: "small", MonthEgressBytes: EgressIncludedGB << 30, EgressBytes: 1, PeriodHours: 720}); r.EgressCents != 1 {
		t.Fatalf("one byte over: %+v", r)
	}
	// The hour that crosses the line pays for the excess only.
	r := Price(Inputs{Class: "small", MonthEgressBytes: 499 << 30, EgressBytes: 3 << 30, PeriodHours: 720})
	if r.EgressCents != 2*EgressPerGB {
		t.Fatalf("threshold hour charged %d cents, want %d", r.EgressCents, 2*EgressPerGB)
	}
}

// A partial hour rounds half up (§5.4).
func TestPartialHourRoundsHalfUp(t *testing.T) {
	for _, c := range []struct {
		class string
		secs  int
		want  int64
	}{
		{"large", 0, 0},
		{"large", 1800, 7},  // exactly half: rounds up
		{"large", 1799, 7},  // 6.996 -> 7
		{"large", 128, 0},   // 0.49 -> 0
		{"large", 129, 1},   // 0.50 -> 1
		{"large", 3600, 14}, // a whole hour
		{"large", 7200, 14}, // a sample overrun is clipped to the hour
		{"small", 1800, 4},  // 3.5 -> 4
		{"xl", 900, 7},      // a quarter of 28
	} {
		if got := Price(Inputs{Class: c.class, RunningSeconds: c.secs, PeriodHours: 720}).GuestCents; got != c.want {
			t.Errorf("%s for %d s: %d cents, want %d", c.class, c.secs, got, c.want)
		}
	}
}
