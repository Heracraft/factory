package scheduler_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/heracraft/repose/internal/api/scheduler"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/db/testdb"
)

func TestMain(m *testing.M) { os.Exit(testdb.Run(m)) }

type host struct {
	name      string
	state     string
	draining  bool
	mem       int64
	free      int64
	pool      int64
	heartbeat time.Time
}

func addHost(t *testing.T, pool *db.Pool, h host) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	_, err := pool.Exec(context.Background(), `insert into hosts (id, name, state, draining, mem_bytes, free_mem_bytes, pool_free_bytes, pool_bytes, last_heartbeat_at) values ($1,$2,$3,$4,$5,$6,$7,$7,$8)`,
		id, h.name, h.state, h.draining, h.mem, h.free, h.pool, h.heartbeat)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func pick(t *testing.T, pool *db.Pool, class string, vol int64) (scheduler.Pick, error) {
	t.Helper()
	var p scheduler.Pick
	err := db.InTx(context.Background(), pool, func(tx pgx.Tx) error {
		var err error
		p, err = scheduler.PickHost(context.Background(), tx, class, vol, time.Now())
		return err
	})
	return p, err
}

func TestPickTable(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name  string
		hosts []host
		class string
		want  string // host name or "" for capacity error
	}{
		{"most free wins", []host{{"a", "ready", false, 64 << 30, 20 << 30, 1 << 40, now}, {"b", "ready", false, 64 << 30, 40 << 30, 1 << 40, now}}, "large", "b"},
		{"draining skipped", []host{{"a", "ready", true, 64 << 30, 50 << 30, 1 << 40, now}, {"b", "ready", false, 64 << 30, 20 << 30, 1 << 40, now}}, "large", "b"},
		{"unreachable skipped", []host{{"a", "unreachable", false, 64 << 30, 50 << 30, 1 << 40, now}, {"b", "ready", false, 64 << 30, 20 << 30, 1 << 40, now}}, "large", "b"},
		{"stale heartbeat skipped", []host{{"a", "ready", false, 64 << 30, 50 << 30, 1 << 40, now.Add(-2 * time.Minute)}, {"b", "ready", false, 64 << 30, 20 << 30, 1 << 40, now}}, "large", "b"},
		{"full host skipped", []host{{"a", "ready", false, 64 << 30, 7 << 30, 1 << 40, now}}, "large", ""},
		{"pool full skipped", []host{{"a", "ready", false, 64 << 30, 50 << 30, 10 << 30, now}}, "large", ""},
		{"reserve respected", []host{{"a", "ready", false, 16 << 30, 16 << 30, 1 << 40, now}}, "xl", ""},
		{"xl fits above reserve", []host{{"a", "ready", false, 32 << 30, 32 << 30, 1 << 40, now}}, "xl", "a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pool := testdb.Open(t)
			for _, h := range c.hosts {
				addHost(t, pool, h)
			}
			p, err := pick(t, pool, c.class, 40<<30)
			if c.want == "" {
				if !errors.Is(err, scheduler.ErrNoCapacity) {
					t.Fatalf("expected capacity error, got %v %+v", err, p)
				}
				return
			}
			if err != nil || p.Name != c.want {
				t.Fatalf("got %+v %v want %s", p, err, c.want)
			}
		})
	}
}

// 100 parallel creates on a host with room for 30 large guests yield
// exactly 30 placements and 70 capacity errors.
func TestConcurrentPlacementsReserveExactly(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	// 256 GB host: reserve 16 GB, 240 GB free for 30 large (8 GB) guests.
	hid := addHost(t, pool, host{"big", "ready", false, 256 << 30, 256 << 30, 1 << 40, time.Now()})
	uid := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx, "insert into users (id, handle) values ($1, 'u')", uid); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	placed, capacity := 0, 0
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := db.InTx(ctx, pool, func(tx pgx.Tx) error {
				p, err := scheduler.PickHost(ctx, tx, "large", 40<<30, time.Now())
				if err != nil {
					return err
				}
				_, err = tx.Exec(ctx, `insert into projects (id, user_id, name, slug, class, state, host_id, volume_bytes) values ($1, $2, $3, $3, 'large', 'creating', $4, 1)`,
					uuid.Must(uuid.NewV7()), uid, uuid.NewString()[:8], p.HostID)
				return err
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				placed++
			case errors.Is(err, scheduler.ErrNoCapacity):
				capacity++
			default:
				t.Errorf("create %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if placed != 30 || capacity != 70 {
		t.Fatalf("placed=%d capacity=%d", placed, capacity)
	}
	var reserved int64
	if err := pool.QueryRow(ctx, "select reserved_bytes from host_reservations where host_id = $1", hid).Scan(&reserved); err != nil {
		t.Fatal(err)
	}
	if reserved != 30*(8<<30) {
		t.Fatalf("reserved %d", reserved)
	}
}
