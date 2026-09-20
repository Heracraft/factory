package billing

import (
	"context"
	"errors"
	"time"
)

// ErrDisabled is returned by every Stripe call while billing is not
// configured (DECISIONS I-16). The HTTP layer maps it to
// `503 billing_disabled`.
var ErrDisabled = errors.New("billing_disabled")

// UsageRow is one usage_hours row as it is pushed to Stripe. The three
// cost parts go to three meters so the invoice carries three lines per
// period (§5.5); the credit has already been taken off, because a negative
// line is not allowed and the trial is consumed before the push.
type UsageRow struct {
	ProjectID  string
	UserID     string
	CustomerID string
	Hour       time.Time
	// The parts after the trial credit was applied, in cents.
	GuestCents   int64
	StorageCents int64
	EgressCents  int64
	// CostCents is the hour's full cost before the credit, for the log line
	// and the reconciliation.
	CostCents   int64
	CreditCents int64
}

// Billable is the amount actually pushed: the hour's cost less whatever
// the trial credit covered.
func (r UsageRow) Billable() int64 { return r.GuestCents + r.StorageCents + r.EgressCents }

// UsagePusher pushes usage records to Stripe. The record id it returns is
// stored on the usage_hours row and a row with one is never pushed again
// (§5.5).
type UsagePusher interface {
	PushUsage(ctx context.Context, row UsageRow) (recordID string, err error)
}

// Disabled is the pusher when Stripe is not configured.
type Disabled struct{}

// PushUsage returns ErrDisabled.
func (Disabled) PushUsage(context.Context, UsageRow) (string, error) { return "", ErrDisabled }

// Portal is the Stripe customer-facing surface behind /billing/*.
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

// Summary is what reconciliation reads back from Stripe for one user and
// one period: the quantity Stripe has recorded against each meter.
type Summary struct {
	GuestCents   int64
	StorageCents int64
	EgressCents  int64
}

// Total is the sum of the three parts.
func (s Summary) Total() int64 { return s.GuestCents + s.StorageCents + s.EgressCents }

// Reader is the read side reconciliation needs (§5.7). The disabled
// implementations do not provide it, so a reconcile without Stripe
// compares usage_hours against itself and says so.
type Reader interface {
	PeriodSummary(ctx context.Context, customerID string, from, to time.Time) (Summary, error)
}
