package httpapi_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/ops"
)

// TestDestroyAndRestartContract is I-156 and I-157 over HTTP: DELETE
// answers 202 {op_id, state} and a repeat while it runs names the same
// op; `start` on a running project whose guestd stopped answering, or on
// one in error, is a restart ({op_id, restart: true}) that ends running.
func TestDestroyAndRestartContract(t *testing.T) {
	e := newEnv(t)
	ctx := e.h.Ctx
	tok := e.signIn(t, "sub-rob", "rob")
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
	// Running with a live guestd: start is a conflict, as before.
	if r := e.do(t, tok, "POST", "/projects/"+pid+"/start", nil); r.status != 409 {
		t.Fatalf("start while running: %d %s", r.status, r.raw)
	}
	// guestd dies; the host's sample says so.
	e.h.Fake.SetGuestdDead(p.GuestID.String(), true)
	if _, err := e.h.Pool.Exec(ctx, "insert into meter_samples (ts, project_id, host_id, state, class, guestd_ok) values (now(), $1, $2, 'running', 'small', false)", pid, p.HostID); err != nil {
		t.Fatal(err)
	}
	r = e.do(t, tok, "POST", "/projects/"+pid+"/start", nil)
	if r.status != 202 || r.body["restart"] != true {
		t.Fatalf("start with guestd dead: %d %s", r.status, r.raw)
	}
	if op := e.waitOp(t, r); op.State != "done" {
		t.Fatalf("restart op: %+v", op.Error)
	}
	if st := e.h.Project(uuid.MustParse(pid)).State; st != "running" || e.h.Fake.Guests()[0].GuestdDead {
		t.Fatalf("after restart: %s", st)
	}
	// In error with guestd dead again: start restarts.
	e.h.Fake.SetGuestdDead(p.GuestID.String(), true)
	if _, err := e.h.Pool.Exec(ctx, "update projects set state = 'error' where id = $1", pid); err != nil {
		t.Fatal(err)
	}
	r = e.do(t, tok, "POST", "/projects/"+pid+"/start", nil)
	if r.status != 202 || r.body["restart"] != true {
		t.Fatalf("start from error: %d %s", r.status, r.raw)
	}
	if op := e.waitOp(t, r); op.State != "done" {
		t.Fatalf("restart from error: %+v", op.Error)
	}
	// Destroy with guestd dead: the op ends done; a repeated DELETE while
	// it is open names the same op.
	e.h.Fake.SetGuestdDead(p.GuestID.String(), true)
	if _, err := e.h.Pool.Exec(ctx, "update projects set state = 'error' where id = $1", pid); err != nil {
		t.Fatal(err)
	}
	e.h.StopEngine() // hold the op open so the repeat sees it
	r = e.do(t, tok, "DELETE", "/projects/"+pid, nil)
	if r.status != 202 || r.body["op_id"] == nil || r.body["state"] != "pending" {
		t.Fatalf("destroy: %d %s", r.status, r.raw)
	}
	again := e.do(t, tok, "DELETE", "/projects/"+pid, nil)
	if again.status != 202 || again.body["op_id"] != r.body["op_id"] {
		t.Fatalf("repeated destroy: %d %s (first %s)", again.status, again.raw, r.raw)
	}
	e.h.StartEngine(ops.Config{})
	if op := e.waitOp(t, r); op.State != "done" {
		t.Fatalf("destroy op: %+v", op.Error)
	}
	if g := e.do(t, tok, "GET", "/projects/"+pid+"/ops/"+r.body["op_id"].(string), nil); g.status != 200 || g.body["state"] != "done" {
		t.Fatalf("op after destroy: %d %s", g.status, g.raw)
	}
	if again := e.do(t, tok, "DELETE", "/projects/"+pid, nil); again.status != 404 {
		t.Fatalf("DELETE of a destroyed project: %d %s", again.status, again.raw)
	}
}
