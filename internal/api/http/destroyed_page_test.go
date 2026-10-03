package httpapi_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/store"
)

// DECISIONS I-420: GET /projects/destroyed pages with limit and before;
// with neither it is the newest 100, as before.
func TestDestroyedPageBack(t *testing.T) {
	e := newEnv(t)
	ctx := e.h.Ctx
	tok := e.signIn(t, "sub-gone", "goner")
	me := e.do(t, tok, "GET", "/me", nil)
	u, err := store.GetUser(ctx, e.h.Pool, uuid.MustParse(me.body["id"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	for i := 0; i < 130; i++ {
		p := e.h.NewProject(u, fmt.Sprintf("gone-%03d", i), "large")
		// Two destroys share each second, so the boundary breaks ties by id.
		at := base.Add(time.Duration(i/2) * time.Second)
		if _, err := e.h.Pool.Exec(ctx, "update projects set state = 'destroyed', destroyed_at = $2 where id = $1", p.ID, at); err != nil {
			t.Fatal(err)
		}
		if _, err := e.h.Pool.Exec(ctx, `insert into snapshots (id, project_id, blob_path, bytes, reason, taken_at, expires_at)
			values ($1, $2, $3, 1, 'stop', $4, now() + interval '30 days')`, uuid.New(), p.ID, "test/"+p.ID.String(), at); err != nil {
			t.Fatal(err)
		}
	}
	if r := e.do(t, tok, "GET", "/projects/destroyed", nil); r.status != 200 || len(r.list) != 100 {
		t.Fatalf("default page: %d, %d rows", r.status, len(r.list))
	}
	seen := map[string]bool{}
	var before, last string
	pages := 0
	for {
		q := "/projects/destroyed?limit=40"
		if before != "" {
			q += "&before=" + before
		}
		r := e.do(t, tok, "GET", q, nil)
		if r.status != 200 {
			t.Fatalf("%s: %d %s", q, r.status, r.raw)
		}
		if len(r.list) == 0 {
			break
		}
		pages++
		for _, it := range r.list {
			m := it.(map[string]any)
			slug := m["slug"].(string)
			if seen[slug] {
				t.Fatalf("%s came twice", slug)
			}
			seen[slug] = true
			if at := m["destroyed_at"].(string); last != "" && at > last {
				t.Fatalf("not newest first: %s after %s", at, last)
			}
			last = m["destroyed_at"].(string)
			before = m["id"].(string)
		}
	}
	if len(seen) != 130 || pages != 4 {
		t.Fatalf("paged %d projects in %d pages, want 130 in 4", len(seen), pages)
	}
	for _, q := range []string{"?limit=0", "?limit=201", "?before=nope"} {
		if r := e.do(t, tok, "GET", "/projects/destroyed"+q, nil); r.status != 400 {
			t.Errorf("%s: %d, want 400", q, r.status)
		}
	}
	// A live project's id, or another user's, pages nothing.
	live := e.h.NewProject(u, "alive", "large")
	if r := e.do(t, tok, "GET", "/projects/destroyed?before="+live.ID.String(), nil); r.status != 200 || len(r.list) != 0 {
		t.Fatalf("live before: %d %d", r.status, len(r.list))
	}
}
