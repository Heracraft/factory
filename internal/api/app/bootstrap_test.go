package app_test

import (
	"context"
	"sync"
	"testing"

	"github.com/heracraft/repose/internal/api/app"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/db/testdb"
	"github.com/heracraft/repose/internal/fakes/kv"
)

// The api bootstraps itself on Coolify (DECISIONS I-90): against an empty
// database it applies the schema and generates the CA, and a second start
// (or a concurrent replica) finds both in place and changes nothing.
func TestBootstrapFromEmptyIsIdempotent(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	if _, err := db.MigrateTo(ctx, pool, 0); err != nil {
		t.Fatalf("emptying the schema: %v", err)
	}
	vault := kv.New()
	cfg := func() app.Config {
		return app.Config{Mode: "all", Listen: freePort(t), GRPCListen: freePort(t), InternalListen: freePort(t), MetricsListen: freePort(t),
			DatabaseURL: pool.Config().ConnString(), Dev: true, Migrate: true, KeyVault: vault,
			APIResource: "https://api.test", GatewayHost: "ssh.test", GatewayPort: 22, ReplicaID: "test", GRPCServerNames: []string{"127.0.0.1"}}
	}

	// Two replicas starting at once against the empty database.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := cfg()
			c.ReplicaID = "r" + string(rune('0'+i))
			_, errs[i] = app.New(ctx, c, "test")
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("replica %d: %v", i, err)
		}
	}
	st, err := db.MigrateStatus(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Pending) != 0 || len(st.Applied) == 0 {
		t.Fatalf("after bootstrap: applied=%d pending=%d", len(st.Applied), len(st.Pending))
	}
	var cas int
	if err := pool.QueryRow(ctx, "select count(*) from secrets where project_id = '00000000-0000-7000-8000-000000000000' and name = 'SSH_USER_CA'").Scan(&cas); err != nil {
		t.Fatal(err)
	}
	if cas != 1 {
		t.Fatalf("expected exactly one user CA secret, got %d", cas)
	}

	// A third start: nothing to migrate, CA already there.
	a, err := app.New(ctx, cfg(), "test")
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if a.HostCA() == nil {
		t.Fatal("restart: CA not loaded")
	}
}
