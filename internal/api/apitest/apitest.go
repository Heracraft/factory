// Package apitest wires a whole control plane in-process for tests: a
// fresh Postgres, an in-memory CA, secrets on the fake Key Vault, hostmgr
// on a loopback mTLS listener, the ops engine, build logs, events and
// meter ingest, and one registered fake host connected over the real
// stream. HTTP tests and ops tests both build on it.
package apitest

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/heracraft/repose/internal/api/buildlog"
	"github.com/heracraft/repose/internal/api/ca"
	"github.com/heracraft/repose/internal/api/events"
	"github.com/heracraft/repose/internal/api/hostmgr"
	"github.com/heracraft/repose/internal/api/meter"
	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/api/ops"
	"github.com/heracraft/repose/internal/api/secrets"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/db/testdb"
	fakehostd "github.com/heracraft/repose/internal/fakes/hostd"
	"github.com/heracraft/repose/internal/fakes/kv"
	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
)

// Harness is the assembled control plane.
type Harness struct {
	T       *testing.T
	Ctx     context.Context
	Pool    *db.Pool
	CA      *ca.CA
	KV      *kv.Fake
	Secrets *secrets.Store
	Metrics *metrics.M
	Log     *slog.Logger
	HostMgr *hostmgr.Server
	Logs    *buildlog.Store
	Events  *events.Ingest
	Meter   *meter.Ingest
	Engine  *ops.Engine

	engine   atomic.Pointer[ops.Engine]
	grpcAddr string
	cancel   context.CancelFunc
	engineCancel context.CancelFunc

	// Host is the registered fake host.
	HostID   uuid.UUID
	Fake     *fakehostd.Fake
	hostCert tls.Certificate
	fakeStop context.CancelFunc
}

// Options tune the harness.
type Options struct {
	BuildDelay time.Duration
	FakeOpts   fakehostd.Options
	// NoHost skips registering and connecting the fake host.
	NoHost bool
}

// New assembles a harness on a fresh database.
func New(t *testing.T, o Options) *Harness {
	t.Helper()
	pool := testdb.Open(t)
	ctx, cancel := context.WithCancel(context.Background())
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if os.Getenv("APITEST_VERBOSE") != "" {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	h := &Harness{T: t, Ctx: ctx, Pool: pool, KV: kv.New(), Metrics: metrics.NewNop(), Log: log, cancel: cancel}
	h.Secrets = secrets.New(pool, h.KV)
	var err error
	h.CA, err = ca.NewInMemory(pool)
	if err != nil {
		t.Fatal(err)
	}
	h.HostMgr = hostmgr.New(pool, h.CA.X509(), "replica-1", h.Metrics, log)
	h.Logs = buildlog.New(pool, log)
	h.Events = events.New(pool, h.Metrics, log)
	h.Meter = meter.New(pool, h.Metrics, log)
	h.HostMgr.SetHandlers(hostmgr.Handlers{
		Hello: func(ctx context.Context, hostID uuid.UUID, hl *hostdv1.Hello) {
			if e := h.engine.Load(); e != nil {
				e.OnHello(ctx, hostID, hl)
			}
		},
		Result: func(ctx context.Context, hostID uuid.UUID, r *hostdv1.Result) {
			if e := h.engine.Load(); e != nil {
				e.OnResult(ctx, hostID, r)
			}
		},
		Samples: h.Meter.OnSamples,
		Event:   h.Events.OnEvent,
		BuildLog: func(ctx context.Context, hostID uuid.UUID, l *hostdv1.BuildLog) {
			if opID, ok := h.Logs.OpFor(l.CommandId); ok {
				h.Logs.Append(opID, int64(l.Seq), l.Line)
			}
		},
	})
	tlsCfg, err := h.HostMgr.TLSConfig(nil, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	gs := h.HostMgr.GRPCServer(tlsCfg)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = gs.Serve(ln) }()
	h.grpcAddr = ln.Addr().String()
	go h.Logs.Run(ctx)
	h.StartEngine(ops.Config{BaseRef: "deadbeef"})
	t.Cleanup(func() {
		cancel()
		gs.Stop()
	})
	if !o.NoHost {
		fo := o.FakeOpts
		if fo.Heartbeat == 0 {
			fo.Heartbeat = 100 * time.Millisecond
		}
		if fo.SampleInterval == 0 {
			fo.SampleInterval = time.Hour
		}
		fo.BuildDelay = o.BuildDelay
		h.ConnectHost(t, "host-01", fo)
	}
	return h
}

// StartEngine replaces the ops engine (simulating an api restart).
func (h *Harness) StartEngine(cfg ops.Config) *ops.Engine {
	if h.engineCancel != nil {
		h.engineCancel()
	}
	ectx, ecancel := context.WithCancel(h.Ctx)
	h.engineCancel = ecancel
	e := ops.New(h.Pool, h.HostMgr, h.CA, h.Secrets, h.Logs, h.Metrics, h.Log, cfg)
	h.engine.Store(e)
	h.Engine = e
	go e.Run(ectx)
	return e
}

// StopEngine halts the engine loop without replacing it.
func (h *Harness) StopEngine() {
	if h.engineCancel != nil {
		h.engineCancel()
	}
	h.engine.Store(nil)
	// The advisory lock is released when the loop's connection returns.
	time.Sleep(50 * time.Millisecond)
}

// ConnectHost registers a fake host and connects its stream.
func (h *Harness) ConnectHost(t *testing.T, name string, fo fakehostd.Options) {
	t.Helper()
	token, err := hostmgr.MintJoinToken(h.Ctx, h.Pool, name, "fake", "fake", "local", false)
	if err != nil {
		t.Fatal(err)
	}
	conn := h.dial(t, nil)
	resp, err := hostdv1.NewHostServiceClient(conn).Register(h.Ctx, &hostdv1.RegisterRequest{JoinToken: token, Info: &hostdv1.HostInfo{Hostname: name, MemBytes: 256 << 30, Vcpus: 64, PoolBytes: 2 << 40}})
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	h.HostID = uuid.MustParse(resp.HostId)
	h.hostCert, err = tls.X509KeyPair(resp.ClientCert, resp.ClientKey)
	if err != nil {
		t.Fatal(err)
	}
	fo.HostID = resp.HostId
	h.Fake = fakehostd.New(fo)
	h.ReconnectHost(t)
}

// ReconnectHost drops the host's stream and opens a new one with the
// same fake (its guests survive, like hostd's bbolt state).
func (h *Harness) ReconnectHost(t *testing.T) {
	t.Helper()
	if h.fakeStop != nil {
		h.fakeStop()
		h.WaitFor("host disconnected", func() bool { return !h.HostMgr.Connected(h.HostID) })
	}
	fctx, fcancel := context.WithCancel(h.Ctx)
	h.fakeStop = fcancel
	conn := h.dial(t, &h.hostCert)
	go func() { _ = h.Fake.Run(fctx, conn) }()
	h.WaitFor("host connected", func() bool {
		hr, err := store.GetHost(h.Ctx, h.Pool, h.HostID)
		return err == nil && (hr.State == "ready" || hr.State == "draining") && hr.LastHeartbeatAt != nil && h.HostMgr.Connected(h.HostID)
	})
}

// DisconnectHost drops the stream without reconnecting.
func (h *Harness) DisconnectHost() {
	if h.fakeStop != nil {
		h.fakeStop()
		h.WaitFor("host disconnected", func() bool { return !h.HostMgr.Connected(h.HostID) })
	}
}

func (h *Harness) dial(t *testing.T, cert *tls.Certificate) *grpc.ClientConn {
	t.Helper()
	cfg := &tls.Config{RootCAs: h.CA.X509().Pool(), ServerName: "127.0.0.1", MinVersion: tls.VersionTLS13}
	if cert != nil {
		cfg.Certificates = []tls.Certificate{*cert}
	}
	conn, err := grpc.NewClient(h.grpcAddr, grpc.WithTransportCredentials(credentials.NewTLS(cfg)))
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

// WaitFor polls cond for up to 15 s.
func (h *Harness) WaitFor(what string, cond func() bool) {
	h.T.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	h.T.Fatalf("timed out waiting for %s", what)
}

// NewUser inserts a user.
func (h *Harness) NewUser(handle string) *store.User {
	h.T.Helper()
	id := store.NewID()
	email := handle + "@example.com"
	if _, err := h.Pool.Exec(h.Ctx, "insert into users (id, logto_sub, handle, email, billing_status, has_card) values ($1, $2, $3, $4, 'trial', true)", id, "sub-"+handle, handle, email); err != nil {
		h.T.Fatal(err)
	}
	u, err := store.GetUser(h.Ctx, h.Pool, id)
	if err != nil {
		h.T.Fatal(err)
	}
	return u
}

// NewProject inserts a project in state creating with a default revision
// and returns it (the HTTP handler's job, done directly here).
func (h *Harness) NewProject(u *store.User, name, class string) *store.Project {
	h.T.Helper()
	pid := store.NewID()
	rid := store.NewID()
	err := db.InTx(h.Ctx, h.Pool, func(tx db.Tx) error {
		if _, err := tx.Exec(h.Ctx, "insert into projects (id, user_id, name, slug, class, state, volume_bytes, config_revision_id) values ($1, $2, $3, $3, $4, 'creating', $5, $6)", pid, u.ID, name, class, int64(40)<<30, rid); err != nil {
			return err
		}
		_, err := tx.Exec(h.Ctx, "insert into config_revisions (id, project_id, fragment, status) values ($1, $2, $3, 'building')", rid, pid, "{ pkgs, ... }: { }")
		return err
	})
	if err != nil {
		h.T.Fatal(err)
	}
	p, err := store.GetProject(h.Ctx, h.Pool, pid)
	if err != nil {
		h.T.Fatal(err)
	}
	return p
}

// Enqueue inserts an op and kicks the engine.
func (h *Harness) Enqueue(n ops.NewOp) uuid.UUID {
	h.T.Helper()
	id, err := h.Engine.Enqueue(h.Ctx, h.Pool, n, false)
	if err != nil {
		h.T.Fatal(err)
	}
	h.Engine.Kick()
	return id
}

// WaitOp waits for an op to finish and returns it.
func (h *Harness) WaitOp(id uuid.UUID) *store.Op {
	h.T.Helper()
	ctx, cancel := context.WithTimeout(h.Ctx, 30*time.Second)
	defer cancel()
	for {
		op, err := store.GetOp(ctx, h.Pool, id)
		if err != nil {
			h.T.Fatal(err)
		}
		if op.State == "done" || op.State == "error" {
			return op
		}
		select {
		case <-ctx.Done():
			h.T.Fatalf("op %s did not finish: state=%s step=%d", id, op.State, op.Step)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// Project reloads a project.
func (h *Harness) Project(id uuid.UUID) *store.Project {
	h.T.Helper()
	p, err := store.GetProject(h.Ctx, h.Pool, id)
	if err != nil {
		h.T.Fatal(err)
	}
	return p
}

// CreateRunning creates a project and drives the create op to running.
func (h *Harness) CreateRunning(u *store.User, name string) *store.Project {
	h.T.Helper()
	p := h.NewProject(u, name, "large")
	pid := p.ID
	op := h.WaitOp(h.Enqueue(ops.NewOp{Kind: ops.KindCreate, ProjectID: &pid, Phases: ops.PlanCreate()}))
	if op.State != "done" {
		h.T.Fatalf("create failed: %+v", op.Error)
	}
	return h.Project(pid)
}
