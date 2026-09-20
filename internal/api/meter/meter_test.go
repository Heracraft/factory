package meter_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/meter"
	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/billing"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/db/testdb"
	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
)

func TestMain(m *testing.M) { os.Exit(testdb.Run(m)) }

func seed(t *testing.T, pool *db.Pool, class string, created time.Time) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	uid, pid, gid := store.NewID(), store.NewID(), store.NewID()
	if _, err := pool.Exec(ctx, "insert into users (id, handle) values ($1, $2)", uid, "u"+uid.String()[24:]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "insert into projects (id, user_id, name, slug, class, state, volume_bytes, guest_id, created_at) values ($1, $2, 'todo', $3, $4, 'running', $5, $6, $7)", pid, uid, "s"+pid.String()[24:], class, int64(40)<<30, gid, created); err != nil {
		t.Fatal(err)
	}
	return pid, gid
}

type pusher struct{ pushed []billing.UsageRow }

func (p *pusher) PushUsage(ctx context.Context, r billing.UsageRow) (string, error) {
	p.pushed = append(p.pushed, r)
	return "ur_" + r.Hour, nil
}

// One large guest running 10 hours of a day with 40 GB and 3 GB egress:
// guest 10 x 14 = 140 cents, storage 40 GB x 10 cents / 720 h over 24 h =
// 13.33 cents (13 whole cents by the remainder rule), egress 0.
func TestIngestAndSyntheticDayRollup(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	m := metrics.NewNop()
	day := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	pid, gid := seed(t, pool, "large", day.Add(-time.Hour))
	ing := meter.New(pool, m, log)
	ing.SetNow(func() time.Time { return day })
	hostID := store.NewID()
	egressPerMinute := int64(3<<30) / 600 // 3 GB spread over the 600 running minutes
	for minute := 0; minute < 24*60; minute++ {
		ts := day.Add(time.Duration(minute) * time.Minute)
		running := minute >= 8*60 && minute < 18*60
		state, tx := "stopped", int64(0)
		if running {
			state, tx = "running", egressPerMinute
		}
		ing.OnSamples(ctx, hostID, &hostdv1.Samples{Ts: ts.Unix(), Guests: []*hostdv1.GuestSample{{
			GuestId: gid.String(), State: state, Class: "large", DiskAllocBytes: 40 << 30, DiskUsedBytes: 6 << 30, NetTxBytesDelta: uint64(tx),
			Signals: &hostdv1.GuestSignals{SshSessions: 1, TmuxClients: 1, GuestdOk: true, Agents: []*hostdv1.AgentProc{{Agent: "claude", TmuxWindow: "claude", State: "working"}}},
			Procs:   []*hostdv1.ProcSample{{Comm: "claude", CpuNsDelta: 1e9, RssBytes: 1 << 30}, {Comm: "node", CpuNsDelta: 2e8, RssBytes: 1 << 20}},
		}}})
	}
	var samples, procs int
	_ = pool.QueryRow(ctx, "select count(*) from meter_samples where project_id = $1", pid).Scan(&samples)
	_ = pool.QueryRow(ctx, "select count(*) from proc_samples where project_id = $1", pid).Scan(&procs)
	if samples != 1440 || procs != 2880 {
		t.Fatalf("samples=%d procs=%d", samples, procs)
	}
	// A re-sent message is harmless.
	ing.OnSamples(ctx, hostID, &hostdv1.Samples{Ts: day.Unix(), Guests: []*hostdv1.GuestSample{{GuestId: gid.String(), State: "stopped", Class: "large"}}})
	_ = pool.QueryRow(ctx, "select count(*) from meter_samples where project_id = $1", pid).Scan(&samples)
	if samples != 1440 {
		t.Fatalf("duplicate sample inserted: %d", samples)
	}
	latest, ok, err := meter.LatestSample(ctx, pool, pid)
	if err != nil || !ok {
		t.Fatalf("latest: %v %v", ok, err)
	}
	_ = latest
	p := &pusher{}
	r := meter.NewRollup(pool, p, m, log)
	r.Now = func() time.Time { return day.Add(25 * time.Hour) }
	var rows []meter.Row
	for h := 0; h < 24; h++ {
		out, err := r.Hour(ctx, day.Add(time.Duration(h)*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, out...)
	}
	if len(rows) != 24 {
		t.Fatalf("rows %d", len(rows))
	}
	var running, guestCents, storageCents, egressCents, cost, egress int64
	if err := pool.QueryRow(ctx, "select sum(running_seconds), sum(guest_cents), sum(storage_cents), sum(egress_cents), sum(cost_cents), sum(egress_bytes) from usage_hours where project_id = $1", pid).Scan(&running, &guestCents, &storageCents, &egressCents, &cost, &egress); err != nil {
		t.Fatal(err)
	}
	if running != 10*3600 {
		t.Fatalf("running seconds %d", running)
	}
	if guestCents != 140 || storageCents != 13 || egressCents != 0 || cost != 153 {
		t.Fatalf("guest=%d storage=%d egress=%d cost=%d", guestCents, storageCents, egressCents, cost)
	}
	if egress < 3<<30-1000 || egress > 3<<30 {
		t.Fatalf("egress bytes %d", egress)
	}
	// Stripe push records the id per row (rows with cost).
	var pushed int
	_ = pool.QueryRow(ctx, "select count(*) from usage_hours where project_id = $1 and stripe_usage_record_id is not null", pid).Scan(&pushed)
	if pushed == 0 || len(p.pushed) != pushed {
		t.Fatalf("pushed %d rows, pusher saw %d", pushed, len(p.pushed))
	}
	// Re-running an hour changes nothing.
	if _, err := r.Hour(ctx, day.Add(9*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var cost2 int64
	_ = pool.QueryRow(ctx, "select sum(cost_cents) from usage_hours where project_id = $1", pid).Scan(&cost2)
	if cost2 != cost {
		t.Fatalf("rollup not idempotent: %d then %d", cost, cost2)
	}
	// A running project with no samples for an hour is a gap with zeros.
	pid2, _ := seed(t, pool, "small", day)
	out, err := r.Hour(ctx, day.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range out {
		if row.ProjectID == pid2 {
			found = true
			if !row.Gap || row.RunningSeconds != 0 || row.GuestCents != 0 {
				t.Fatalf("gap row %+v", row)
			}
		}
	}
	if !found {
		t.Fatal("project without samples missing from the rollup")
	}
	// Due rolls up everything up to the previous full hour.
	pool2 := testdb.Open(t)
	ing2 := meter.New(pool2, m, log)
	_, gid2 := seed(t, pool2, "xl", day)
	ing2.OnSamples(ctx, hostID, &hostdv1.Samples{Ts: day.Add(30 * time.Minute).Unix(), Guests: []*hostdv1.GuestSample{{GuestId: gid2.String(), State: "running", Class: "xl"}}})
	r2 := meter.NewRollup(pool2, billing.Disabled{}, m, log)
	r2.Now = func() time.Time { return day.Add(3*time.Hour + 5*time.Minute) }
	n, err := r2.Due(ctx)
	if err != nil || n != 3 {
		t.Fatalf("due rolled %d hours: %v", n, err)
	}
	if n, _ := r2.Due(ctx); n != 0 {
		t.Fatalf("second due rolled %d", n)
	}
}
