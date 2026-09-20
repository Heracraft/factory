package billing

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/obs"
)

// BILLING_ENFORCE (09-billing.md §8): turning billing off keeps rolling up
// and pushing but stops blocking starts and stopping guests, "and must be
// logged as an audit event when flipped". The value the api last ran with
// is kept in `settings`, so a deploy that changes it writes one audit_log
// row rather than one per start.

// SettingEnforce is the settings key holding the last observed value.
const SettingEnforce = "billing_enforce"

// RecordEnforcement compares enforce with the stored value and, when it
// differs, writes the audit_log row and the log line. It returns whether
// the value changed.
func RecordEnforcement(ctx context.Context, pool *db.Pool, enforce bool, actor string, log *slog.Logger) (bool, error) {
	prev, err := store.Setting(ctx, pool, SettingEnforce)
	if err != nil {
		return false, err
	}
	now := strconv.FormatBool(enforce)
	if prev == now {
		return false, nil
	}
	// Compare and set in one statement: two replicas starting together
	// would otherwise both read the old value and both write an audit row
	// for one flip.
	tag, err := pool.Exec(ctx, `insert into settings (key, value) values ($1, $2)
		on conflict (key) do update set value = excluded.value where settings.value is distinct from excluded.value`,
		SettingEnforce, now)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := store.Audit(ctx, pool, actor, "billing_enforce", "", map[string]any{"from": prev, "to": now}); err != nil {
		return false, err
	}
	if enforce {
		log.Info("billing enforcement on: starts are gated and past-due guests stop", "event", obs.EventBillingEnforce, "enforced", true)
	} else {
		log.Warn("BILLING_ENFORCE=false: usage is still metered and pushed, but no start is blocked and no guest is stopped", "event", obs.EventBillingEnforce, "enforced", false)
	}
	return true, nil
}
