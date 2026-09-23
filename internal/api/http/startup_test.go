package httpapi_test

import (
	"testing"

	"github.com/google/uuid"
)

// I-225: the newest sample may predate the guest's current run (taken
// while it was being stopped, guestd already gone). It says nothing about
// the guestd running now: start is not a restart, and the project shows
// guestd_ok as unknown, until a sample from after the start arrives.
func TestSampleFromBeforeTheStartIsNotGuestdDead(t *testing.T) {
	e := newEnv(t)
	ctx := e.h.Ctx
	tok := e.signIn(t, "sub-start", "starter")
	r := e.do(t, tok, "POST", "/projects", map[string]any{"name": "age-calculator", "class": "small"})
	if r.status/100 != 2 {
		t.Fatalf("create: %d %s", r.status, r.raw)
	}
	pid := r.body["id"].(string)
	var createOp string
	if err := e.h.Pool.QueryRow(ctx, "select id::text from ops where project_id = $1 and kind = 'create'", pid).Scan(&createOp); err != nil {
		t.Fatal(err)
	}
	if op := e.h.WaitOp(uuid.MustParse(createOp)); op.State != "done" {
		t.Fatalf("create op: %+v", op.Error)
	}
	e.h.WaitIdle(uuid.MustParse(pid))
	p := e.h.Project(uuid.MustParse(pid))
	if p.StartedAt == nil {
		t.Fatal("started_at not set on a running project")
	}
	// The sample of the run before: running, guestd gone, a minute before
	// this run's start.
	if _, err := e.h.Pool.Exec(ctx, "insert into meter_samples (ts, project_id, host_id, state, class, guestd_ok) values ($3::timestamptz - interval '1 minute', $1, $2, 'running', 'small', false)", pid, p.HostID, *p.StartedAt); err != nil {
		t.Fatal(err)
	}
	g := e.do(t, tok, "GET", "/projects/"+pid, nil)
	sig, _ := g.body["signals"].(map[string]any)
	if sig == nil {
		t.Fatalf("no signals: %s", g.raw)
	}
	if v, present := sig["guestd_ok"]; !present || v != nil {
		t.Fatalf("guestd_ok = %v (present %v), want null", v, present)
	}
	if r := e.do(t, tok, "POST", "/projects/"+pid+"/start", nil); r.status != 409 {
		t.Fatalf("start with only a stale sample: %d %s, want 409 (no restart)", r.status, r.raw)
	}
	// A sample from this run that says guestd is dead still counts.
	if _, err := e.h.Pool.Exec(ctx, "insert into meter_samples (ts, project_id, host_id, state, class, guestd_ok) values (now(), $1, $2, 'running', 'small', false)", pid, p.HostID); err != nil {
		t.Fatal(err)
	}
	g = e.do(t, tok, "GET", "/projects/"+pid, nil)
	if sig, _ := g.body["signals"].(map[string]any); sig == nil || sig["guestd_ok"] != false {
		t.Fatalf("guestd_ok after a fresh dead sample: %s", g.raw)
	}
	e.h.Fake.SetGuestdDead(p.GuestID.String(), true)
	if r := e.do(t, tok, "POST", "/projects/"+pid+"/start", nil); r.status != 202 || r.body["restart"] != true {
		t.Fatalf("start with a fresh dead sample: %d %s", r.status, r.raw)
	}
}
