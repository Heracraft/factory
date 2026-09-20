package buildlog_test

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/api/buildlog"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db/testdb"
)

func TestMain(m *testing.M) { os.Exit(testdb.Run(m)) }

// A build line that echoes a current secret value is stored with the
// value replaced, and subscribers see the redacted line too.
func TestRedactionBatchingAndSubscribe(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	uid, pid, opID := store.NewID(), store.NewID(), store.NewID()
	if _, err := pool.Exec(ctx, "insert into users (id, handle) values ($1, 'bl')", uid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "insert into projects (id, user_id, name, slug, class, state, volume_bytes) values ($1, $2, 'p', 'p', 'small', 'running', 1)", pid, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "insert into ops (id, project_id, kind, state) values ($1, $2, 'build', 'running')", opID, pid); err != nil {
		t.Fatal(err)
	}
	s := buildlog.New(pool, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	s.SetRedactions(opID, []string{"sk-live-PLANTED", "x"}) // short values are never redacted (too many false hits)
	ch, cancel := s.Subscribe(opID)
	defer cancel()
	for i := 1; i <= 120; i++ {
		line := "building"
		if i == 7 {
			line = "export API_KEY=sk-live-PLANTED # leaked by a build hook"
		}
		s.Append(opID, int64(i), line)
	}
	s.Flush(ctx)
	lines, err := s.Read(ctx, opID, 0, 0)
	if err != nil || len(lines) != 120 {
		t.Fatalf("read %d %v", len(lines), err)
	}
	if lines[6].Line != "export API_KEY=[redacted] # leaked by a build hook" {
		t.Fatalf("line 7 stored as %q", lines[6].Line)
	}
	var stored string
	if err := pool.QueryRow(ctx, "select line from build_logs where op_id = $1 and seq = 7", opID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "PLANTED") {
		t.Fatal("secret value reached the table")
	}
	got := 0
	timeout := time.After(2 * time.Second)
	for got < 120 {
		select {
		case l := <-ch:
			got++
			if strings.Contains(l.Line, "PLANTED") {
				t.Fatal("secret value reached a subscriber")
			}
		case <-timeout:
			t.Fatalf("subscriber saw %d of 120 lines", got)
		}
	}
	// A partial tail read.
	tail, _ := s.Read(ctx, opID, 118, 0)
	if len(tail) != 2 || tail[0].Seq != 119 {
		t.Fatalf("tail %+v", tail)
	}
	s.ClearRedactions(opID)
	if n, err := s.Trim(ctx, 20); err != nil || n != 0 {
		t.Fatalf("trim %d %v", n, err)
	}
}
