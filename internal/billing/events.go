package billing

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/store"
)

// The account events of DECISIONS I-291: an events row with user_id and
// no project, plus one email outbox row, in the caller's transaction, the
// way waitlist.Admit does it. They are transactional (sent whatever
// notify_email says), because each answers something the user did or is
// about to be charged for.
const (
	KindTrialEnding           = "trial_ending"
	KindPaymentFailed         = "payment_failed"
	KindSubscriptionCancelled = "subscription_cancelled"
	KindSubscriptionEnded     = "subscription_ended"
	KindPlanChanged           = "plan_changed"
	KindEgressStopped         = "egress_stopped"
	// KindBillingStopped is the per-project event the 3-day stop records
	// (13-notifications.md §5.6); it predates I-291 and keeps its name.
	KindBillingStopped = "billing_stopped"
)

// AccountKinds lists the user-only event kinds this package emits, for
// the notifier's transactional list.
var AccountKinds = []string{KindTrialEnding, KindPaymentFailed, KindSubscriptionCancelled, KindSubscriptionEnded, KindPlanChanged, KindEgressStopped}

// AccountEvent records one account event and queues its email. The
// summary is the sentence the email and the dashboard show; it carries
// amounts, dates and plan names, never anything from inside a guest.
func AccountEvent(ctx context.Context, q store.Querier, userID uuid.UUID, at time.Time, kind, summary string) (uuid.UUID, error) {
	id := store.NewID()
	at = at.UTC()
	if _, err := q.Exec(ctx, `insert into events (id, project_id, user_id, ts, ts_second, kind, summary, source) values ($1, null, $2, $3, $4, $5, $6, 'api')`,
		id, userID, at, at.Unix(), kind, summary); err != nil {
		return uuid.Nil, err
	}
	var email *string
	if err := q.QueryRow(ctx, "select email from users where id = $1", userID).Scan(&email); err != nil {
		return uuid.Nil, err
	}
	if email != nil && *email != "" {
		if _, err := q.Exec(ctx, "insert into events_outbox (event_id, channel) values ($1, 'email')", id); err != nil {
			return uuid.Nil, err
		}
	}
	return id, nil
}

// accountEventCount counts the user's events of a kind at or after since.
func accountEventCount(ctx context.Context, q store.Querier, userID uuid.UUID, kind string, since time.Time) (int, error) {
	var n int
	err := q.QueryRow(ctx, "select count(*) from events where user_id = $1 and kind = $2 and ts >= $3", userID, kind, since.UTC()).Scan(&n)
	return n, err
}
