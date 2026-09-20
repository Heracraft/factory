package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/heracraft/repose/internal/api/store"
)

// The trial credit ledger (09-billing.md §5.3). Sign-up inserts +1000
// "trial"; each hour's cost first debits the ledger until the balance is
// zero and only the remainder becomes a Stripe usage record. The balance is
// sum(cents) over the user's rows, never a cached column: users.
// trial_credit_cents is a projection maintained by a trigger inside the
// same transaction (migration 0003), so the two can never drift.

// Reasons a credit_ledger row carries.
const (
	ReasonTrial      = "trial"
	ReasonUsage      = "usage"
	ReasonGoodwill   = "goodwill"
	ReasonRefund     = "refund"
	ReasonAdjustment = "adjustment"
)

// Entry is one credit_ledger row.
type Entry struct {
	ID        uuid.UUID `db:"id"`
	UserID    uuid.UUID `db:"user_id"`
	Cents     int64     `db:"cents"`
	Reason    string    `db:"reason"`
	Ref       *string   `db:"ref"`
	CreatedAt time.Time `db:"created_at"`
}

// UsageRef is the ledger reference of a usage debit: the usage_hours
// primary key, as §5.3 asks ("a negative row per hour with ref =
// usage_hours pk").
func UsageRef(projectID uuid.UUID, hour time.Time) string {
	return projectID.String() + ":" + hour.UTC().Format(time.RFC3339)
}

// Balance is the user's credit balance in cents. It reads the ledger, not
// the projection, so a caller that wants the truth gets it.
func Balance(ctx context.Context, q store.Querier, userID uuid.UUID) (int64, error) {
	var cents int64
	err := q.QueryRow(ctx, "select coalesce(sum(cents), 0) from credit_ledger where user_id = $1", userID).Scan(&cents)
	return cents, err
}

// Credit adds a ledger row. cents may be negative; reason and ref are
// recorded as given. It is the operator path behind `repose-admin billing
// credit` and the webhook path for a refund.
func Credit(ctx context.Context, q store.Querier, userID uuid.UUID, cents int64, reason, ref string) (uuid.UUID, error) {
	if reason == "" {
		return uuid.Nil, errors.New("credit: a reason is required")
	}
	id := store.NewID()
	var refp *string
	if ref != "" {
		refp = &ref
	}
	_, err := q.Exec(ctx, "insert into credit_ledger (id, user_id, cents, reason, ref) values ($1, $2, $3, $4, $5)", id, userID, cents, reason, refp)
	if err != nil {
		return uuid.Nil, fmt.Errorf("credit %s %d: %w", reason, cents, err)
	}
	return id, nil
}

// ListEntries returns a user's ledger, newest first.
func ListEntries(ctx context.Context, q store.Querier, userID uuid.UUID, limit int) ([]Entry, error) {
	rows, err := q.Query(ctx, "select id, user_id, cents, reason, ref, created_at from credit_ledger where user_id = $1 order by created_at desc, id desc limit $2", userID, limit)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByName[Entry])
	if out == nil {
		out = []Entry{}
	}
	return out, err
}

// DebitUsage applies an hour's cost against the trial credit inside the
// caller's transaction. The user row is locked first, so two rollup
// runners cannot both see the same balance (§6, "trial balance negative
// (race): impossible by construction").
//
// A re-run of the same hour finds the existing debit for the reference: if
// the recomputed cost wants the same debit, nothing is written and the
// rollup stays idempotent; if it wants a different one, an 'adjustment'
// row carries the difference, because the ledger is append-only.
// It returns the total debited for the hour, as a positive number.
func DebitUsage(ctx context.Context, tx store.Querier, userID, projectID uuid.UUID, hour time.Time, costCents int64) (int64, error) {
	ref := UsageRef(projectID, hour)
	var locked uuid.UUID
	if err := tx.QueryRow(ctx, "select id from users where id = $1 for update", userID).Scan(&locked); err != nil {
		return 0, fmt.Errorf("lock user for the credit debit: %w", err)
	}
	var already int64
	if err := tx.QueryRow(ctx, "select coalesce(sum(cents), 0) from credit_ledger where ref = $1 and reason in ('usage','adjustment')", ref).Scan(&already); err != nil {
		return 0, fmt.Errorf("read the existing debit: %w", err)
	}
	already = -already // stored negative, wanted positive
	// The balance available to this hour excludes what this hour already
	// took, so a re-run does not see its own debit as someone else's.
	var balance int64
	if err := tx.QueryRow(ctx, "select coalesce(sum(cents), 0) from credit_ledger where user_id = $1", userID).Scan(&balance); err != nil {
		return 0, fmt.Errorf("read the credit balance: %w", err)
	}
	available := balance + already
	if available < 0 {
		available = 0
	}
	want := costCents
	if want > available {
		want = available
	}
	if want < 0 {
		want = 0
	}
	switch {
	case want == already:
		// Nothing to write, but a re-run of the hour that exhausted the
		// credit must still leave the account on the right status.
		return already, endOfTrial(ctx, tx, userID)
	case already == 0:
		if _, err := Credit(ctx, tx, userID, -want, ReasonUsage, ref); err != nil {
			return 0, err
		}
	default:
		if _, err := Credit(ctx, tx, userID, already-want, ReasonAdjustment, ref); err != nil {
			return 0, err
		}
	}
	return want, endOfTrial(ctx, tx, userID)
}

// endOfTrial moves an account off `trial` once its credit is gone
// (09-billing.md §5.3: "when the balance hits zero they become active and
// the next hour is billed"; PRICING.md: "nothing stops"). It runs inside
// the debit's transaction, under the same row lock.
//
// The failure it prevents: the card gate refuses a start with
// `trial_depleted` for any user whose status is `trial` and whose balance
// is zero, so an account that simply used its ten dollars would have been
// locked out of its own guests instead of being charged for them.
func endOfTrial(ctx context.Context, tx store.Querier, userID uuid.UUID) error {
	_, err := tx.Exec(ctx, `update users set billing_status = 'active'
		where id = $1 and billing_status = 'trial' and has_card
		and coalesce((select sum(cents) from credit_ledger where user_id = $1), 0) <= 0`, userID)
	if err != nil {
		return fmt.Errorf("end the trial: %w", err)
	}
	return nil
}
