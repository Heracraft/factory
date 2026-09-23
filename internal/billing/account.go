package billing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v83"

	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
)

// Account is one user's billing state in one place: what `repose-admin
// billing show` prints, and what the M4 gate runbook reads its evidence
// from (docs/ops/M4-GATE.md). The period totals are what the invoice for
// that period must come to: usage_hours cost less the credit, split the
// way PushUsage splits it.
type Account struct {
	Handle       string
	UserID       uuid.UUID
	Status       string
	HasCard      bool
	Balance      int64
	Customer     string
	Subscription string
	Anchor       *time.Time
	Period       Period
	PastDueSince *time.Time
	SuspendedAt  *time.Time
	ProjectLimit int
	XLLimit      int

	Hours          int
	CostCents      int64
	CreditCents    int64
	ComputeBilled  int64
	StorageBilled  int64
	EgressBilled   int64
	UnpushedRows   int
	Invoices       []AccountInvoice
	FirstBilledHrs *time.Time
}

// AccountInvoice is one invoices row.
type AccountInvoice struct {
	StripeID    string
	Status      string
	TotalCents  int64
	PeriodStart *time.Time
	PeriodEnd   *time.Time
}

// Billed is what the period's invoice must total before tax.
func (a Account) Billed() int64 { return a.ComputeBilled + a.StorageBilled + a.EgressBilled }

// LoadAccount reads the account and its current period (the one containing
// at) from the database alone.
func LoadAccount(ctx context.Context, pool *db.Pool, userID uuid.UUID, at time.Time) (Account, error) {
	u, err := store.GetUser(ctx, pool, userID)
	if err != nil {
		return Account{}, err
	}
	a := Account{Handle: u.Handle, UserID: u.ID, Status: u.BillingStatus, HasCard: u.HasCard, Anchor: u.BillingAnchor,
		PastDueSince: u.PastDueSince, SuspendedAt: u.SuspendedAt, ProjectLimit: u.ProjectLimit, XLLimit: u.XLLimit}
	if u.StripeCustomerID != nil {
		a.Customer = *u.StripeCustomerID
	}
	if u.StripeSubscriptionID != nil {
		a.Subscription = *u.StripeSubscriptionID
	}
	anchor := u.CreatedAt
	if u.BillingAnchor != nil {
		anchor = *u.BillingAnchor
	}
	a.Period = PeriodFor(anchor, at)
	if a.Balance, err = Balance(ctx, pool, u.ID); err != nil {
		return a, err
	}
	rows, err := pool.Query(ctx, `select u.guest_cents, u.storage_cents, u.egress_cents, u.cost_cents, u.credit_cents,
		u.stripe_usage_record_id is null and u.cost_cents > u.credit_cents, u.hour
		from usage_hours u join projects p on p.id = u.project_id
		where p.user_id = $1 and u.period_start = $2 order by u.hour`, u.ID, a.Period.Start)
	if err != nil {
		return a, err
	}
	for rows.Next() {
		var g, s, e, cost, credit int64
		var unpushed bool
		var hour time.Time
		if err := rows.Scan(&g, &s, &e, &cost, &credit, &unpushed, &hour); err != nil {
			rows.Close()
			return a, err
		}
		a.Hours++
		a.CostCents += cost
		a.CreditCents += credit
		g, s, e = applyCredit(g, s, e, credit)
		a.ComputeBilled += g
		a.StorageBilled += s
		a.EgressBilled += e
		if unpushed {
			a.UnpushedRows++
		}
		if cost > credit && a.FirstBilledHrs == nil {
			h := hour.UTC()
			a.FirstBilledHrs = &h
		}
	}
	rows.Close()
	irows, err := pool.Query(ctx, `select stripe_invoice_id, status, total_cents, period_start, period_end from invoices
		where user_id = $1 order by coalesce(period_end, created_at) desc limit 6`, u.ID)
	if err != nil {
		return a, err
	}
	defer irows.Close()
	for irows.Next() {
		var inv AccountInvoice
		if err := irows.Scan(&inv.StripeID, &inv.Status, &inv.TotalCents, &inv.PeriodStart, &inv.PeriodEnd); err != nil {
			return a, err
		}
		a.Invoices = append(a.Invoices, inv)
	}
	return a, irows.Err()
}

func fmtT(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

// WriteTo prints the account as a two-column table.
func (a Account) WriteTo(w io.Writer) (int64, error) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	row := func(k, v string) { _, _ = fmt.Fprintf(tw, "%s\t%s\n", k, v) }
	row("handle", a.Handle)
	row("billing", a.Status)
	row("has_card", fmt.Sprint(a.HasCard))
	row("credit balance", fmt.Sprintf("%d cents (from credit_ledger)", a.Balance))
	row("limits", fmt.Sprintf("%d projects, %d xl", a.ProjectLimit, a.XLLimit))
	row("stripe customer", orDash(a.Customer))
	row("stripe subscription", orDash(a.Subscription))
	row("billing anchor", fmtT(a.Anchor))
	row("past due since", fmtT(a.PastDueSince))
	row("suspended", fmtT(a.SuspendedAt))
	row("period", a.Period.Start.Format(time.RFC3339)+" to "+a.Period.End.Format(time.RFC3339))
	row("  usage_hours rows", fmt.Sprint(a.Hours))
	row("  cost", fmt.Sprintf("%d cents", a.CostCents))
	row("  trial credit applied", fmt.Sprintf("%d cents", a.CreditCents))
	row("  billed", fmt.Sprintf("compute %d + storage %d + egress %d = %d cents (the invoice before tax)", a.ComputeBilled, a.StorageBilled, a.EgressBilled, a.Billed()))
	row("  first billed hour", fmtT(a.FirstBilledHrs))
	row("  rows not yet pushed", fmt.Sprint(a.UnpushedRows))
	for i, inv := range a.Invoices {
		k := ""
		if i == 0 {
			k = "invoices"
		}
		row(k, fmt.Sprintf("%s %s %d cents %s to %s", inv.StripeID, inv.Status, inv.TotalCents, fmtT(inv.PeriodStart), fmtT(inv.PeriodEnd)))
	}
	return 0, tw.Flush()
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// ErrNoSubscription means the account has no Stripe subscription yet.
var ErrNoSubscription = errors.New("the account has no Stripe subscription (no card was ever attached)")

// CycleNow ends the account's current billing period now: Stripe resets
// the subscription's billing cycle anchor, which invoices the usage so far
// immediately, and the rollup's anchor moves with it so the next hour
// starts a new period on both sides. It is the operator's way to get a
// real invoice for a short known pattern without waiting a month
// (DECISIONS I-185, docs/ops/M4-GATE.md). The caller pushes every finished
// hour first and waits for Stripe to have them. The hour holding the reset
// goes to the new period on both sides: the rollup's anchor is that hour,
// and its events are stamped at its last second, after the reset.
func (s *Stripe) CycleNow(ctx context.Context, userID uuid.UUID) (invoiceID string, anchor time.Time, err error) {
	u, err := store.GetUser(ctx, s.pool, userID)
	if err != nil {
		return "", anchor, err
	}
	if u.StripeSubscriptionID == nil || *u.StripeSubscriptionID == "" {
		return "", anchor, ErrNoSubscription
	}
	params := &stripe.SubscriptionUpdateParams{BillingCycleAnchorNow: stripe.Bool(true), ProrationBehavior: str("none")}
	params.AddExpand("latest_invoice")
	sub, err := s.c.V1Subscriptions.Update(ctx, *u.StripeSubscriptionID, params)
	if err != nil {
		return "", anchor, fmt.Errorf("reset the billing cycle: %w", err)
	}
	anchor = s.Now().UTC()
	if sub.BillingCycleAnchor > 0 {
		anchor = time.Unix(sub.BillingCycleAnchor, 0).UTC()
	}
	anchor = anchor.Truncate(time.Hour)
	if _, err := s.pool.Exec(ctx, "update users set billing_anchor = $2 where id = $1", u.ID, anchor); err != nil {
		return "", anchor, err
	}
	if sub.LatestInvoice != nil {
		invoiceID = sub.LatestInvoice.ID
	}
	return invoiceID, anchor, nil
}
