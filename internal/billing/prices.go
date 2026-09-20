// Package billing holds the price table from docs/PRICING.md and the
// per-hour cost function the api's rollup calls (05-control-plane-api.md
// §5.10). Workstream 09 owns the Stripe side; the api needs the numbers
// and the cap rule to write usage_hours from the first hour.
package billing

import (
	"context"
	"errors"
)

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

// Inputs is one hour of one project plus the period running totals.
type Inputs struct {
	Class          string
	RunningSeconds int
	GBAlloc        int64
	EgressBytes    int64
	// PeriodHours is the billing period's length; 720 when unknown.
	PeriodHours int
	// MonthGuestCents is the guest part already charged this period.
	MonthGuestCents int64
	// MonthEgressBytes is the egress already counted this period, before
	// this hour.
	MonthEgressBytes int64
	// StorageRemainder carries the fractional storage cost between hours,
	// in units of 1/(1000*PeriodHours) of a cent, so a period sums exactly
	// to gb_alloc * 10.
	StorageRemainder int64
}

// Result is the priced hour.
type Result struct {
	CostCents        int64
	GuestCents       int64
	StorageCents     int64
	EgressCents      int64
	StorageRemainder int64
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
	if cap := Cap(in.Class); in.MonthGuestCents+r.GuestCents > cap {
		r.GuestCents = cap - in.MonthGuestCents
		if r.GuestCents < 0 {
			r.GuestCents = 0
		}
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

// ErrDisabled is returned by every Stripe call while billing is not
// configured (DECISIONS I-16).
var ErrDisabled = errors.New("billing_disabled")

// UsageRow is what the rollup pushes.
type UsageRow struct {
	ProjectID string
	UserID    string
	Hour      string
	CostCents int64
}

// UsagePusher pushes usage records to Stripe. Workstream 09 provides the
// real one; Disabled stands in until STRIPE_* is configured.
type UsagePusher interface {
	PushUsage(ctx context.Context, row UsageRow) (recordID string, err error)
}

// Disabled is the pusher when Stripe is not configured.
type Disabled struct{}

// PushUsage returns ErrDisabled.
func (Disabled) PushUsage(context.Context, UsageRow) (string, error) { return "", ErrDisabled }

// Portal is the Stripe customer-facing surface behind /billing/*.
// Workstream 09 provides the real one.
type Portal interface {
	PortalURL(ctx context.Context, userID string) (string, error)
	SetupIntent(ctx context.Context, userID string) (clientSecret string, err error)
	Invoices(ctx context.Context, userID string) ([]map[string]any, error)
}

// DisabledPortal returns ErrDisabled for everything.
type DisabledPortal struct{}

// PortalURL returns ErrDisabled.
func (DisabledPortal) PortalURL(context.Context, string) (string, error) { return "", ErrDisabled }

// SetupIntent returns ErrDisabled.
func (DisabledPortal) SetupIntent(context.Context, string) (string, error) { return "", ErrDisabled }

// Invoices returns ErrDisabled.
func (DisabledPortal) Invoices(context.Context, string) ([]map[string]any, error) {
	return nil, ErrDisabled
}
