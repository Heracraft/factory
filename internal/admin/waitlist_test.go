package admin_test

import (
	"strings"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/admin"
	"github.com/heracraft/repose/internal/api/apitest"
)

// DECISIONS I-269: `waitlist list` shows the queue, `waitlist admit`
// lets one user or the next N in, audited, with one email each.
func TestWaitlistCommands(t *testing.T) {
	h := apitest.New(t, apitest.Options{NoHost: true})
	e := &admin.Env{KV: h.KV, Actor: "admin:test"}
	e.SetPool(h.Pool)
	now := time.Now()
	for i, handle := range []string{"ann", "bob", "cyd"} {
		u := h.NewUser(handle)
		if _, err := h.Pool.Exec(h.Ctx, "insert into waitlist (user_id, joined_at) values ($1, $2)", u.ID, now.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	out, err := run(t, e, "waitlist", "list")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[1], "1 ") || !strings.Contains(lines[1], "ann") || !strings.Contains(lines[3], "cyd") {
		t.Fatalf("list:\n%s", out)
	}
	if out, err := run(t, e, "waitlist", "admit", "bob"); err != nil || !strings.Contains(out, "bob admitted") {
		t.Fatalf("admit bob: %s %v", out, err)
	}
	if out, err := run(t, e, "waitlist", "admit", "bob"); err != nil || !strings.Contains(out, "bob was already admitted") {
		t.Fatalf("admit bob again: %s %v", out, err)
	}
	if out, err := run(t, e, "waitlist", "admit", "--next", "5"); err != nil || !strings.Contains(out, "ann admitted") || !strings.Contains(out, "cyd admitted") {
		t.Fatalf("admit --next: %s %v", out, err)
	}
	if out, err := run(t, e, "waitlist", "admit", "--next", "1"); err != nil || !strings.Contains(out, "nobody is waiting") {
		t.Fatalf("admit --next on an empty queue: %s %v", out, err)
	}
	if _, err := run(t, e, "waitlist", "admit"); err == nil {
		t.Fatal("admit with neither a handle nor --next was accepted")
	}
	if _, err := run(t, e, "waitlist", "admit", "ann", "--next", "1"); err == nil {
		t.Fatal("admit with both a handle and --next was accepted")
	}
	var audits, emails int
	if err := h.Pool.QueryRow(h.Ctx, "select (select count(*) from audit_log where action = 'waitlist_admit' and actor = 'admin:test'), (select count(*) from events_outbox o join events ev on ev.id = o.event_id where ev.kind = 'waitlist_admitted')").Scan(&audits, &emails); err != nil {
		t.Fatal(err)
	}
	if audits != 3 || emails != 3 {
		t.Fatalf("audits %d emails %d, want 3 and 3", audits, emails)
	}
	out, _ = run(t, e, "waitlist", "list")
	if !strings.Contains(out, "admin:test") || strings.Contains(out, "\n1 ") {
		t.Fatalf("list after admission:\n%s", out)
	}
}
