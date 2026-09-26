package httpapi_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	httpapi "github.com/heracraft/repose/internal/api/http"
	"github.com/heracraft/repose/internal/api/notify"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/api/waitlist"
)

// DECISIONS I-269: a first project waits while the fleet's reserved memory
// would pass the line; the place is kept on retry; users with a project,
// admitted users and exempt accounts are never gated; the admitter lets
// the queue in oldest first while the projection fits, one email each,
// however often and by however many admitters it runs.
func TestWaitlistGateAndAdmission(t *testing.T) {
	var gate *waitlist.Gate
	e := newEnvWith(t, &httpapi.RateLimits{General: 10000, Certs: 10000, Config: 10000}, func(d *httpapi.Deps) {
		gate = &waitlist.Gate{Pool: d.Pool, Percent: 80, M: d.Metrics}
		d.Waitlist = gate
	})
	ctx := e.h.Ctx
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := e.h.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	// The harness host contributes no usable memory (8 GB is all reserve);
	// a second host of 48 GB has 40 GB usable, so the line is 32 GB.
	exec("update hosts set mem_bytes = $2 where id = $1", e.h.HostID, int64(8)<<30)
	hostID := store.NewID()
	exec("insert into hosts (id, name, state, mem_bytes) values ($1, 'host-wl', 'ready', $2)", hostID, int64(48)<<30)
	filler := e.h.NewUser("filler")
	var fillers []uuid.UUID
	for i := 0; i < 4; i++ { // 4 large: 32 GB reserved, at the line
		id := store.NewID()
		fillers = append(fillers, id)
		exec("insert into projects (id, user_id, name, slug, class, state, volume_bytes, host_id) values ($1, $2, $3, $3, 'large', 'running', $4, $5)",
			id, filler.ID, "fill"+string(rune('a'+i)), int64(40)<<30, hostID)
	}

	// A new user's first project is refused and waitlisted.
	tokB := e.signIn(t, "sub-wl-b", "wlb")
	r := e.do(t, tokB, "POST", "/projects", map[string]any{"name": "first", "class": "small"})
	if r.status != 503 {
		t.Fatalf("first create: %d %s", r.status, r.raw)
	}
	errObj, _ := r.body["error"].(map[string]any)
	detail, _ := errObj["detail"].(map[string]any)
	if errObj["code"] != "waitlisted" || detail["position"] != float64(1) || detail["email"] != "wlb@example.com" {
		t.Fatalf("refusal: %s", r.raw)
	}
	if msg, _ := errObj["message"].(string); msg != "repose is at capacity. You're number 1 on the waitlist; we'll email wlb@example.com when there's room." {
		t.Fatalf("message %q", msg)
	}
	joined := detail["joined_at"]
	// A retry keeps the place and the join time.
	r = e.do(t, tokB, "POST", "/projects", map[string]any{"name": "first", "class": "large"})
	errObj, _ = r.body["error"].(map[string]any)
	detail, _ = errObj["detail"].(map[string]any)
	if r.status != 503 || detail["position"] != float64(1) || detail["joined_at"] != joined {
		t.Fatalf("retry: %d %s", r.status, r.raw)
	}
	// GET /me shows the place.
	me := e.do(t, tokB, "GET", "/me", nil)
	wl, _ := me.body["waitlist"].(map[string]any)
	if wl["position"] != float64(1) {
		t.Fatalf("me: %s", me.raw)
	}
	idB := uuid.MustParse(me.body["id"].(string))
	tokC := e.signIn(t, "sub-wl-c", "wlc")
	r = e.do(t, tokC, "POST", "/projects", map[string]any{"name": "first"})
	errObj, _ = r.body["error"].(map[string]any)
	detail, _ = errObj["detail"].(map[string]any)
	if errObj["code"] != "waitlisted" || detail["position"] != float64(2) {
		t.Fatalf("second waiter: %s", r.raw)
	}
	var rows int
	if err := e.h.Pool.QueryRow(ctx, "select count(*) from waitlist").Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("waitlist rows %d %v", rows, err)
	}
	if !strings.Contains(e.logs.String(), `"event":"waitlist_join"`) || strings.Contains(e.logs.String(), "wlb@example.com") {
		t.Fatalf("join log: %s", e.logs.String())
	}

	// A user who has a project, and an exempt account, are never gated;
	// with the gate off nobody is.
	if w, _, err := gate.Check(ctx, filler, "xl", time.Now()); err != nil || w != nil {
		t.Fatalf("user with projects gated: %v %v", w, err)
	}
	staff := e.h.NewUser("staff")
	exec("update users set billing_status = 'exempt' where id = $1", staff.ID)
	staff, _ = store.GetUser(ctx, e.h.Pool, staff.ID)
	if w, _, err := gate.Check(ctx, staff, "large", time.Now()); err != nil || w != nil {
		t.Fatalf("exempt gated: %v %v", w, err)
	}
	fresh := e.h.NewUser("fresh")
	off := &waitlist.Gate{Pool: e.h.Pool, Percent: 0}
	if w, _, err := off.Check(ctx, fresh, "large", time.Now()); err != nil || w != nil {
		t.Fatalf("gate off still gated: %v %v", w, err)
	}

	// No room: nobody admitted.
	adm := &waitlist.Admitter{Pool: e.h.Pool, Percent: 80, M: e.h.Metrics}
	now := time.Now()
	if n, err := adm.Run(ctx, now); err != nil || n != 0 {
		t.Fatalf("full fleet admitted %d %v", n, err)
	}
	// 8 GB frees: the next user fits (24 + 8 = 32), the one after would
	// not with that admission counted (24 + 8 + 8 = 40).
	exec("update projects set state = 'stopped' where id = $1", fillers[0])
	if n, err := adm.Run(ctx, now); err != nil || n != 1 {
		t.Fatalf("admitted %d %v, want 1", n, err)
	}
	b, err := store.GetWaitlistEntry(ctx, e.h.Pool, idB)
	if err != nil || b.AdmittedAt == nil || b.AdmittedBy == nil || *b.AdmittedBy != "auto" {
		t.Fatalf("B not admitted: %+v %v", b, err)
	}
	c, err := store.GetWaitlistEntry(ctx, e.h.Pool, uuid.MustParse(e.do(t, tokC, "GET", "/me", nil).body["id"].(string)))
	if err != nil || c.AdmittedAt != nil || c.Position != 1 {
		t.Fatalf("C: %+v %v", c, err)
	}
	// Again, and from a second admitter (another replica, a restart):
	// nothing more, one email.
	if n, err := adm.Run(ctx, now.Add(time.Minute)); err != nil || n != 0 {
		t.Fatalf("rerun admitted %d %v", n, err)
	}
	if n, err := (&waitlist.Admitter{Pool: e.h.Pool, Percent: 80}).Run(ctx, now.Add(2*time.Minute)); err != nil || n != 0 {
		t.Fatalf("second admitter admitted %d %v", n, err)
	}
	if ok, err := waitlist.Admit(ctx, e.h.Pool, idB, "admin:test", now); err != nil || ok {
		t.Fatalf("re-admit: %v %v", ok, err)
	}
	var evs, outbox int
	if err := e.h.Pool.QueryRow(ctx, "select count(*), (select count(*) from events_outbox o join events e2 on e2.id = o.event_id where e2.user_id = $1) from events where user_id = $1 and kind = 'waitlist_admitted'", idB).Scan(&evs, &outbox); err != nil || evs != 1 || outbox != 1 {
		t.Fatalf("events %d outbox %d %v", evs, outbox, err)
	}

	// The email goes out through the outbox even with notify_email off,
	// with no unsubscribe link.
	exec("update users set notify_email = false where id = $1", idB)
	var got []notify.Message
	sender := notify.SenderFunc(func(_ context.Context, m notify.Message) error {
		got = append(got, m)
		return nil
	})
	o := notify.New(e.h.Pool, map[string]notify.Sender{"email": sender}, e.h.Metrics, e.h.Log)
	o.Unsub = e.unsub
	if _, err := o.Once(ctx); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != waitlist.Kind || got[0].Email != "wlb@example.com" || got[0].Project != "" || got[0].Unsubscribe != "" || got[0].Summary != waitlist.Summary {
		t.Fatalf("sent %+v", got)
	}
	if n, _ := o.Once(ctx); n != 0 {
		t.Fatalf("outbox resent %d", n)
	}

	// Admitted, B is past the gate for good; C still waits, and a
	// newcomer queues behind C even though room has appeared.
	if w, _, err := gate.Check(ctx, &store.User{ID: idB, BillingStatus: "trial"}, "large", time.Now()); err != nil || w != nil {
		t.Fatalf("admitted user gated: %v %v", w, err)
	}
	exec("update projects set state = 'stopped' where id = any($1)", fillers[1:])
	tokD := e.signIn(t, "sub-wl-d", "wld")
	r = e.do(t, tokD, "POST", "/projects", map[string]any{"name": "first", "class": "small"})
	errObj, _ = r.body["error"].(map[string]any)
	detail, _ = errObj["detail"].(map[string]any)
	if errObj["code"] != "waitlisted" || detail["position"] != float64(2) {
		t.Fatalf("newcomer behind the queue: %s", r.raw)
	}
	if n, err := adm.Run(ctx, now.Add(3*time.Minute)); err != nil || n != 2 {
		t.Fatalf("admitted %d %v, want C and D", n, err)
	}
	if ws, err := store.ListWaiting(ctx, e.h.Pool); err != nil || len(ws) != 0 {
		t.Fatalf("still waiting: %+v %v", ws, err)
	}
	// D, admitted, creates.
	r = e.do(t, tokD, "POST", "/projects", map[string]any{"name": "first", "class": "small"})
	if r.status != 201 {
		t.Fatalf("admitted create: %d %s", r.status, r.raw)
	}
}
