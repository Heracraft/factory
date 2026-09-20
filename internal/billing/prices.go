// Package billing turns meter samples into money: the price table from
// docs/PRICING.md, the billing period anchored at signup, the hourly
// rollup into usage_hours, the trial credit ledger, the Stripe customer,
// subscription, meter events and webhooks, dunning, and the
// reconciliation that proves usage_hours and Stripe agree
// (docs/workstreams/09-billing.md).
//
// usage_hours is the ledger of record for what was used; Stripe is the
// ledger of record for what was charged; `repose-admin billing reconcile`
// compares them and never fixes a difference silently.
package billing

// Prices in cents, USD (docs/PRICING.md, 09-billing.md §5.1).
const (
	CapSmall  int64 = 4900
	CapLarge  int64 = 9900
	CapXL     int64 = 19900
	HourSmall int64 = 7  // ceil(4900/720)
	HourLarge int64 = 14 // ceil(9900/720)
	HourXL    int64 = 28 // ceil(19900/720)

	StoragePerGBMonth  int64 = 10
	EgressIncludedGB   int64 = 500
	EgressPerGB        int64 = 5
	TrialCreditCents   int64 = 1000
	DefaultPeriodHours       = 720
	ProjectLimitTrial        = 3
	XLLimitTrial             = 1
	ProjectLimitPaid         = 10
	XLLimitPaid              = 10
)

// PriceVersion stamps every usage_hours row. A price change creates a new
// version from the effective hour so historical usage keeps its historical
// price (docs/PRICING.md, "Changing prices"); rows are never repriced.
const PriceVersion = "v1"

// Cap is the monthly cap for a class.
func Cap(class string) int64 {
	switch class {
	case "small":
		return CapSmall
	case "xl":
		return CapXL
	default:
		return CapLarge
	}
}

// Hourly is the hourly rate for a class.
func Hourly(class string) int64 {
	switch class {
	case "small":
		return HourSmall
	case "xl":
		return HourXL
	default:
		return HourLarge
	}
}

// classRank orders the size classes so the larger of two can be named.
func classRank(class string) int {
	switch class {
	case "small":
		return 1
	case "large":
		return 2
	case "xl":
		return 3
	}
	return 0
}

// LargerClass returns whichever of two classes has the larger cap, which is
// the cap a project that changed class mid-period is held to (§5.1). An
// empty or unknown class loses to a known one.
func LargerClass(a, b string) string {
	if classRank(b) > classRank(a) {
		return b
	}
	if classRank(a) == 0 {
		return ""
	}
	return a
}

// Inputs is one hour of one project plus the period running totals.
type Inputs struct {
	Class          string
	RunningSeconds int
	GBAlloc        int64
	EgressBytes    int64
	// CapClass is the largest class the project has run in this period,
	// including this hour: a project that changes class mid-period is
	// capped at the sum of hours_in_class * hourly bounded by the larger
	// class's cap (§5.1). Empty means Class.
	CapClass string
	// PeriodHours is the billing period's length; 720 when unknown.
	PeriodHours int
	// MonthGuestCents is the guest part already charged this period.
	MonthGuestCents int64
	// MonthEgressBytes is the egress already counted this period, before
	// this hour.
	MonthEgressBytes int64
	// StorageRemainder carries the fractional storage cost between hours,
	// in units of 1/(1000*PeriodHours) of a cent, so a period sums exactly
	// to gb_alloc * 10. It starts at zero in a period's first hour.
	StorageRemainder int64
}

// Result is the priced hour.
type Result struct {
	CostCents        int64
	GuestCents       int64
	StorageCents     int64
	EgressCents      int64
	StorageRemainder int64
	// CapApplied is set when the cap clipped the guest part this hour.
	CapApplied bool
	// CapCents is the cap the guest part was held to.
	CapCents int64
}

// Price computes the hour's cost per 09-billing.md §5.4.
func Price(in Inputs) Result {
	var r Result
	// Guest part, rounded half up, then capped on the period running total.
	secs := int64(in.RunningSeconds)
	if secs > 3600 {
		secs = 3600
	}
	r.GuestCents = (Hourly(in.Class)*secs + 1800) / 3600
	capClass := in.CapClass
	if capClass == "" {
		capClass = in.Class
	}
	r.CapCents = Cap(capClass)
	if in.MonthGuestCents+r.GuestCents > r.CapCents {
		r.GuestCents = r.CapCents - in.MonthGuestCents
		if r.GuestCents < 0 {
			r.GuestCents = 0
		}
		r.CapApplied = true
	}
	// Storage: gb_alloc * 10 cents per period, spread per hour; the
	// remainder is the exact rational left over, so the period sums to
	// gb_alloc * 10 with no rounding drift.
	hours := int64(in.PeriodHours)
	if hours <= 0 {
		hours = DefaultPeriodHours
	}
	unit := 1000 * hours
	acc := in.GBAlloc*StoragePerGBMonth*1000 + in.StorageRemainder
	r.StorageCents = acc / unit
	r.StorageRemainder = acc % unit
	// Egress: only the bytes over the included allowance are charged, on
	// the period running total.
	included := EgressIncludedGB << 30
	before := in.MonthEgressBytes
	after := before + in.EgressBytes
	if after > included {
		over := after - included
		if before > included {
			over = after - before
		}
		r.EgressCents = (over*EgressPerGB + (1<<30 - 1)) >> 30
	}
	r.CostCents = r.GuestCents + r.StorageCents + r.EgressCents
	return r
}
