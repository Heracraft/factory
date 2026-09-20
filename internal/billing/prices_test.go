package billing

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The tiers table in docs/PRICING.md and the constants must agree.
func TestPricesMatchPricingDoc(t *testing.T) {
	f, err := os.Open("../../docs/PRICING.md")
	if err != nil {
		t.Skip("PRICING.md not found")
	}
	defer func() { _ = f.Close() }()
	want := map[string][2]int64{"small": {HourSmall, CapSmall}, "large": {HourLarge, CapLarge}, "xl": {HourXL, CapXL}}
	seen := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		cells := strings.Split(sc.Text(), "|")
		if len(cells) < 7 {
			continue
		}
		class := strings.TrimSpace(cells[1])
		w, ok := want[class]
		if !ok {
			continue
		}
		hourly := strings.TrimPrefix(strings.TrimSpace(cells[5]), "$")
		cap := strings.TrimPrefix(strings.TrimSpace(cells[6]), "$")
		h, err1 := strconv.ParseFloat(hourly, 64)
		c, err2 := strconv.ParseFloat(cap, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		if int64(h*100+0.5) != w[0] || int64(c*100+0.5) != w[1] {
			t.Errorf("%s: doc says %s/%s, code says %d/%d", class, hourly, cap, w[0], w[1])
		}
		seen++
	}
	if seen != 3 {
		t.Fatalf("found %d tier rows in PRICING.md", seen)
	}
}

func TestPriceHour(t *testing.T) {
	// A full large hour with 40 GB: 14 cents guest, 40*10/720 = 0.555 cents storage carried forward.
	r := Price(Inputs{Class: "large", RunningSeconds: 3600, GBAlloc: 40, PeriodHours: 720})
	if r.GuestCents != 14 || r.StorageCents != 0 || r.StorageRemainder != 400000 || r.EgressCents != 0 {
		t.Fatalf("%+v", r)
	}
	// Half an hour rounds half up: 7.
	if r := Price(Inputs{Class: "large", RunningSeconds: 1800, PeriodHours: 720}); r.GuestCents != 7 {
		t.Fatalf("half hour %+v", r)
	}
	// Cap: the 720th hour is clipped so the period equals the cap.
	if r := Price(Inputs{Class: "large", RunningSeconds: 3600, PeriodHours: 720, MonthGuestCents: 9890}); r.GuestCents != 10 {
		t.Fatalf("cap %+v", r)
	}
	if r := Price(Inputs{Class: "large", RunningSeconds: 3600, PeriodHours: 720, MonthGuestCents: 9900}); r.GuestCents != 0 {
		t.Fatalf("over cap %+v", r)
	}
	// Storage over a 720-hour period sums exactly to gb * 10.
	var total, rem int64
	for i := 0; i < 720; i++ {
		r := Price(Inputs{Class: "small", GBAlloc: 40, PeriodHours: 720, StorageRemainder: rem})
		total += r.StorageCents
		rem = r.StorageRemainder
	}
	if total != 400 {
		t.Fatalf("storage over a period: %d", total)
	}
	// Egress: the threshold hour charges only the excess.
	r = Price(Inputs{Class: "xl", MonthEgressBytes: 499 << 30, EgressBytes: 2 << 30, PeriodHours: 720})
	if r.EgressCents != 5 {
		t.Fatalf("threshold hour %+v", r)
	}
	r = Price(Inputs{Class: "xl", MonthEgressBytes: 600 << 30, EgressBytes: 3 << 30, PeriodHours: 720})
	if r.EgressCents != 15 {
		t.Fatalf("over threshold %+v", r)
	}
}
