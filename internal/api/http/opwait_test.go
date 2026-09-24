package httpapi_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	httpapi "github.com/heracraft/repose/internal/api/http"
)

// opWaitEnv is a signed-in user with a project whose create finished and
// a start op the test drives by hand: the engine is stopped, so only the
// test's updates move the op and the project.
func opWaitEnv(t *testing.T) (e *env, tok, pid, opID string) {
	t.Helper()
	e = newEnv(t)
	tok = e.signIn(t, "sub-wait", "waiter")
	r := e.do(t, tok, "POST", "/projects", map[string]any{"name": "wait", "class": "small"})
	pid = r.body["id"].(string)
	e.waitOp(t, r)
	e.h.StopEngine()
	opID = uuid.NewString()
	if _, err := e.h.Pool.Exec(e.h.Ctx, `insert into ops (id, project_id, kind, state, params) values ($1, $2, 'start', 'running', '{"phases":["start_guest","apply_config"]}')`, opID, pid); err != nil {
		t.Fatal(err)
	}
	if _, err := e.h.Pool.Exec(e.h.Ctx, `update projects set state = 'starting' where id = $1`, pid); err != nil {
		t.Fatal(err)
	}
	return e, tok, pid, opID
}

func (e *env) sqlAfter(t *testing.T, d time.Duration, sql string, args ...any) {
	t.Helper()
	go func() {
		time.Sleep(d)
		if _, err := e.h.Pool.Exec(e.h.Ctx, sql, args...); err != nil {
			t.Error(err)
		}
	}()
}

func TestOpWaitReturnsOnChange(t *testing.T) {
	e, tok, pid, opID := opWaitEnv(t)
	path := "/projects/" + pid + "/ops/" + opID

	// Without wait: the old answer plus the new fields, no header.
	r := e.do(t, tok, "GET", path, nil)
	if r.status != 200 || r.body["state"] != "running" || r.body["phase"] != "start_guest" || r.body["project_state"] != "starting" {
		t.Fatalf("plain read: %d %s", r.status, r.raw)
	}
	if r.hdr.Get(httpapi.LongPollHeader) != "" {
		t.Fatal("plain read carries the long-poll header")
	}
	v0 := r.body["version"].(string)

	// A phase change wakes the wait.
	e.sqlAfter(t, 300*time.Millisecond, `update ops set step = 1 where id = $1`, opID)
	start := time.Now()
	r = e.do(t, tok, "GET", path+"?wait=10s&seen="+v0, nil)
	el := time.Since(start)
	if r.status != 200 || r.body["phase"] != "apply_config" || r.hdr.Get(httpapi.LongPollHeader) != "1" {
		t.Fatalf("phase wait: %d %s %v", r.status, r.raw, r.hdr)
	}
	if el < 250*time.Millisecond || el > 2*time.Second {
		t.Fatalf("phase wait took %s; want about 300 ms", el)
	}
	v1 := r.body["version"].(string)
	if v1 == v0 {
		t.Fatal("version did not change with the phase")
	}

	// The op finishing (and the project running) wakes it too.
	e.sqlAfter(t, 200*time.Millisecond, `update projects set state = 'running' where id = $1`, pid)
	r = e.do(t, tok, "GET", path+"?wait=10s&seen="+v1, nil)
	if r.body["project_state"] != "running" || r.body["state"] != "running" {
		t.Fatalf("project state wait: %s", r.raw)
	}
	e.sqlAfter(t, 200*time.Millisecond, `update ops set state = 'done', finished_at = now() where id = $1`, opID)
	start = time.Now()
	r = e.do(t, tok, "GET", path+"?wait=10s", nil)
	if r.body["state"] != "done" || r.body["phase"] != nil || time.Since(start) > 2*time.Second {
		t.Fatalf("done wait: %s after %s", r.raw, time.Since(start))
	}

	// A finished op answers at once, with the header.
	start = time.Now()
	r = e.do(t, tok, "GET", path+"?wait=10s&seen="+r.body["version"].(string), nil)
	if r.body["state"] != "done" || r.hdr.Get(httpapi.LongPollHeader) != "1" || time.Since(start) > time.Second {
		t.Fatalf("finished op: %s %v after %s", r.raw, r.hdr, time.Since(start))
	}
}

func TestOpWaitSeenStaleAnswersAtOnce(t *testing.T) {
	e, tok, pid, opID := opWaitEnv(t)
	start := time.Now()
	r := e.do(t, tok, "GET", "/projects/"+pid+"/ops/"+opID+"?wait=10s&seen=pending.0.stopped", nil)
	if r.status != 200 || r.hdr.Get(httpapi.LongPollHeader) != "1" || time.Since(start) > time.Second {
		t.Fatalf("stale seen: %d %s after %s", r.status, r.raw, time.Since(start))
	}
}

func TestOpWaitTimesOut(t *testing.T) {
	e, tok, pid, opID := opWaitEnv(t)
	start := time.Now()
	r := e.do(t, tok, "GET", "/projects/"+pid+"/ops/"+opID+"?wait=700ms", nil)
	el := time.Since(start)
	if r.status != 200 || r.body["state"] != "running" || r.hdr.Get(httpapi.LongPollHeader) != "1" {
		t.Fatalf("timeout: %d %s", r.status, r.raw)
	}
	if el < 650*time.Millisecond || el > 2*time.Second {
		t.Fatalf("wait=700ms took %s", el)
	}
	// Whole seconds are accepted; garbage is invalid.
	if r := e.do(t, tok, "GET", "/projects/"+pid+"/ops/"+opID+"?wait=soon", nil); r.status != 400 || errCode(r) != "invalid" {
		t.Fatalf("wait=soon: %d %s", r.status, r.raw)
	}
	start = time.Now()
	if r := e.do(t, tok, "GET", "/projects/"+pid+"/ops/"+opID+"?wait=1", nil); r.status != 200 || time.Since(start) < 900*time.Millisecond {
		t.Fatalf("wait=1: %d after %s", r.status, time.Since(start))
	}
}

func TestOpWaitAuth(t *testing.T) {
	e, tok, pid, opID := opWaitEnv(t)
	path := "/projects/" + pid + "/ops/" + opID + "?wait=5s"
	if r := e.do(t, "", "GET", path, nil); r.status != 401 {
		t.Fatalf("no token: %d", r.status)
	}
	other := e.signIn(t, "sub-other", "other")
	start := time.Now()
	if r := e.do(t, other, "GET", path, nil); r.status != 404 || time.Since(start) > time.Second {
		t.Fatalf("another user's op: %d after %s", r.status, time.Since(start))
	}
	_ = tok
}

func TestOpWaitBoundedPerUser(t *testing.T) {
	e, tok, pid, opID := opWaitEnv(t)
	path := "/projects/" + pid + "/ops/" + opID
	var wg sync.WaitGroup
	for i := 0; i < httpapi.OpWaitersPerUser; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.do(t, tok, "GET", path+"?wait=3s", nil)
		}()
	}
	time.Sleep(500 * time.Millisecond) // all of them held
	start := time.Now()
	r := e.do(t, tok, "GET", path+"?wait=3s", nil)
	if r.status != 200 || r.hdr.Get(httpapi.LongPollHeader) != "" || time.Since(start) > time.Second {
		t.Fatalf("over the bound: %d %v after %s", r.status, r.hdr, time.Since(start))
	}
	// Another user is not affected by this one's waiters.
	other := e.signIn(t, "sub-other2", "other2")
	r2 := e.do(t, other, "POST", "/projects", map[string]any{"name": "w2", "class": "small"})
	if r2.status != 201 && r2.status != 200 {
		t.Fatalf("second project: %d %s", r2.status, r2.raw)
	}
	var id2 string
	if err := e.h.Pool.QueryRow(e.h.Ctx, "select id::text from ops where project_id = $1", r2.body["id"]).Scan(&id2); err != nil {
		t.Fatal(err)
	}
	if _, err := e.h.Pool.Exec(e.h.Ctx, "update ops set state = 'running' where id = $1", id2); err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	r = e.do(t, other, "GET", "/projects/"+r2.body["id"].(string)+"/ops/"+id2+"?wait=600ms", nil)
	if r.hdr.Get(httpapi.LongPollHeader) != "1" || time.Since(start) < 500*time.Millisecond {
		t.Fatalf("other user: %v after %s", r.hdr, time.Since(start))
	}
	wg.Wait()
	// The slots are released: a wait holds again.
	start = time.Now()
	r = e.do(t, tok, "GET", path+"?wait=600ms", nil)
	if r.hdr.Get(httpapi.LongPollHeader) != "1" || time.Since(start) < 500*time.Millisecond {
		t.Fatalf("after release: %v after %s", r.hdr, time.Since(start))
	}
}

func TestOpWaitAnswersWhenDraining(t *testing.T) {
	e, tok, pid, opID := opWaitEnv(t)
	go func() {
		time.Sleep(300 * time.Millisecond)
		e.srv.SetReady(false)
	}()
	start := time.Now()
	r := e.do(t, tok, "GET", "/projects/"+pid+"/ops/"+opID+"?wait=10s", nil)
	if r.status != 200 || r.body["state"] != "running" || time.Since(start) > 2*time.Second {
		t.Fatalf("drain: %d %s after %s", r.status, r.raw, time.Since(start))
	}
}
