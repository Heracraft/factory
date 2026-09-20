package admin_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/admin"
	"github.com/heracraft/repose/internal/api/apitest"
	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/billing"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/obs"
)

// docs/workstreams/09-billing.md §9: "repose-admin billing
// credit|suspend|unsuspend|reconcile|explain exist and write audit_log."
func TestBillingSubcommands(t *testing.T) {
	h := apitest.New(t, apitest.Options{})
	e := &admin.Env{KV: h.KV, Actor: "admin:test"}
	e.SetPool(h.Pool)
	ctx := h.Ctx

	// An account with a project and an hour of usage to explain.
	uid, pid, gid := store.NewID(), store.NewID(), store.NewID()
	hour := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Hour)
	anchor := hour.AddDate(0, 0, -1)
	if _, err := h.Pool.Exec(ctx, `insert into users (id, handle, email, billing_status, has_card, stripe_customer_id, created_at, billing_anchor)
		values ($1, 'payer', 'payer@example.test', 'trial', true, 'cus_payer', $2, $2)`, uid, anchor); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Pool.Exec(ctx, `insert into projects (id, user_id, name, slug, class, state, volume_bytes, guest_id, created_at)
		values ($1, $2, 'ledger', 'ledger', 'large', 'running', $3, $4, $5)`, pid, uid, int64(40)<<30, gid, anchor); err != nil {
		t.Fatal(err)
	}
	if err := db.EnsurePartitions(ctx, h.Pool, hour); err != nil {
		t.Fatal(err)
	}
	for m := 0; m < 60; m++ {
		if _, err := h.Pool.Exec(ctx, `insert into meter_samples (ts, project_id, host_id, state, class, disk_alloc, net_tx, guestd_ok)
			values ($1, $2, $3, 'running', 'large', $4, 0, true)`, hour.Add(time.Duration(m)*time.Minute), pid, store.NewID(), int64(40)<<30); err != nil {
			t.Fatal(err)
		}
	}

	// rollup --hour prices the hour and prints the table.
	out, err := run(t, e, "billing", "rollup", "--hour", hour.Format("2006-01-02T15"))
	if err != nil || !strings.Contains(out, pid.String()) || !strings.Contains(out, "CREDIT") {
		t.Fatalf("rollup --hour: %s %v", out, err)
	}

	// credit adds a ledger row and reports the new balance.
	out, err = run(t, e, "billing", "credit", "payer", "250", "goodwill after an outage")
	if err != nil || !strings.Contains(out, "+250") || !strings.Contains(out, "balance is now 250") {
		t.Fatalf("credit: %s %v", out, err)
	}
	balance, err := billing.Balance(ctx, h.Pool, uid)
	if err != nil {
		t.Fatal(err)
	}
	if balance != 250 {
		t.Fatalf("balance %d", balance)
	}

	// explain prints the arithmetic of one hour.
	out, err = run(t, e, "billing", "explain", "ledger", hour.Format("2006-01-02T15"))
	if err != nil || !strings.Contains(out, "steps") || !strings.Contains(out, "guest      14 * 3600") {
		t.Fatalf("explain: %s %v", out, err)
	}
	t.Logf("explain:\n%s", out)

	// reconcile without Stripe configured says so rather than reporting a
	// mismatch against zero.
	t.Setenv("STRIPE_SECRET_KEY", "")
	if _, err := run(t, e, "billing", "reconcile"); err == nil {
		t.Fatal("reconcile without Stripe should refuse")
	}

	// resync reports the pending push backlog.
	out, err = run(t, e, "billing", "resync")
	if err != nil || !strings.Contains(out, "pending") {
		t.Fatalf("resync: %s %v", out, err)
	}

	// suspend and unsuspend are the billing spellings of the user actions.
	if out, err := run(t, e, "billing", "suspend", "payer"); err != nil || !strings.Contains(out, "suspended") {
		t.Fatalf("billing suspend: %s %v", out, err)
	}
	var status string
	if err := h.Pool.QueryRow(ctx, "select billing_status from users where id = $1", uid).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "suspended" {
		t.Fatalf("status after billing suspend: %s", status)
	}
	if out, err := run(t, e, "billing", "unsuspend", "payer"); err != nil || !strings.Contains(out, "unsuspended") {
		t.Fatalf("billing unsuspend: %s %v", out, err)
	}

	// Every one of them wrote audit_log.
	for _, action := range []string{"billing_rollup", "billing_credit", "billing_resync", "user_suspend", "user_unsuspend"} {
		var n int
		if err := h.Pool.QueryRow(ctx, "select count(*) from audit_log where action = $1 and actor = 'admin:test'", action).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Errorf("%s wrote no audit_log row", action)
		}
	}
	// The credit row carries the audit id it was made under, so a ledger
	// entry can be traced to the operator who added it.
	var ref *string
	if err := h.Pool.QueryRow(ctx, "select ref from credit_ledger where user_id = $1 and reason = $2", uid, billing.ReasonGoodwill).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	if ref == nil || !strings.HasPrefix(*ref, "audit:") {
		t.Fatalf("credit ref %v", ref)
	}

	// The rollup the admin CLI builds is the same one the api runs.
	r := billing.NewRollup(h.Pool, billing.Disabled{}, metrics.NewNop(), obs.NewLogger(obs.LogOptions{Component: obs.ComponentAdmin}))
	if _, err := r.Hour(context.Background(), hour); err != nil {
		t.Fatal(err)
	}
}
