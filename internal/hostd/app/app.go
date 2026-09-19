// Package app wires hostd together: identity, state, the guest Manager,
// the api stream, the samples loop, certificate rotation, metrics, console
// capture and the operator control socket.
package app

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/heracraft/repose/internal/hostd/ch"
	"github.com/heracraft/repose/internal/hostd/console"
	"github.com/heracraft/repose/internal/hostd/control"
	"github.com/heracraft/repose/internal/hostd/gcroot"
	"github.com/heracraft/repose/internal/hostd/guest"
	"github.com/heracraft/repose/internal/hostd/hostinfo"
	"github.com/heracraft/repose/internal/hostd/lvm"
	"github.com/heracraft/repose/internal/hostd/metrics"
	hnet "github.com/heracraft/repose/internal/hostd/net"
	"github.com/heracraft/repose/internal/hostd/nixbuild"
	"github.com/heracraft/repose/internal/hostd/register"
	"github.com/heracraft/repose/internal/hostd/shell"
	"github.com/heracraft/repose/internal/hostd/snapshot"
	"github.com/heracraft/repose/internal/hostd/state"
	"github.com/heracraft/repose/internal/hostd/stream"
	"github.com/heracraft/repose/internal/hostd/systemd"
	"github.com/heracraft/repose/internal/hostd/vsockclient"
)

// Options are the daemon's flags.
type Options struct {
	Version       string
	StateDir      string
	GuestsDir     string
	BuildsDir     string
	BaseDir       string
	BaseRepoURL   string
	GCRootsDir    string
	APIAddr       string
	APIServerName string
	APICA         string
	TokenPath     string
	MetricsAddr   string
	ControlSock   string
	GuestdUnix    bool
	SnapshotDir   string
	BlobURL       string
	BlobContainer string
	BlobIdentity  string
	StoreExport   string
	VirtiofsUser  string
	VG            string
	Pool          string
	MaxOps        int
	MaxBuilds     int
	FailAtStep    int
	NoWG          bool
	Substituters  string
}

// Logger builds the JSON logger every component shares: ts, level, msg,
// component on every line.
func Logger() *slog.Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo, ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			a.Key = "ts"
		}
		return a
	}})
	return slog.New(h)
}

// TokenError is returned by EnsureIdentity when the token is refused.
var TokenError = register.ErrTokenUsed

// EnsureIdentity loads the identity or registers with the join token,
// retrying every 30 s while the token is missing or the api unreachable.
func EnsureIdentity(ctx context.Context, o Options, log *slog.Logger, r shell.Runner, l lvm.LVM) (*register.Identity, error) {
	if id, err := register.Load(o.StateDir); err == nil {
		return id, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	roots, err := register.LoadRoots(o.APICA)
	if err != nil {
		return nil, err
	}
	cfg := register.Config{Dir: o.StateDir, TokenPath: o.TokenPath, APIAddr: o.APIAddr, ServerName: o.APIServerName, Roots: roots, Runner: r, WGUnit: "wg-quick-wg0.service"}
	if o.NoWG {
		cfg.Runner = nil
	}
	for {
		cfg.Info = hostinfo.Collect(ctx, r, l)
		id, err := register.Register(ctx, cfg)
		switch {
		case err == nil:
			log.Info("registered", "component", "hostd", "event", "register", "host_id", id.Host.HostID)
			return id, nil
		case errors.Is(err, register.ErrTokenUsed):
			log.Error("register: join token already used", "component", "hostd", "event", "register")
			return nil, err
		case errors.Is(err, register.ErrNoToken):
			log.Warn("waiting for join token", "component", "hostd", "event", "register")
		default:
			log.Warn("register failed; retrying", "component", "hostd", "event", "register", "err", err.Error())
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(30 * time.Second):
		}
	}
}

// Daemon is the running hostd.
type Daemon struct {
	o       Options
	log     *slog.Logger
	st      *state.DB
	mgr     *guest.Manager
	strm    *stream.Stream
	metrics *metrics.M
	id      *register.Identity
	idMu    sync.Mutex
	start   time.Time
	dialer  *rotatingDialer
}

type rotatingDialer struct {
	mu    sync.Mutex
	inner stream.GRPCDialer
}

// lateHost lets the stream be built before the Manager.
type lateHost struct{ m *guest.Manager }

func (l *lateHost) Hello() *hello         { return l.m.Hello() }
func (l *lateHost) Heartbeat() *heartbeat { return l.m.Heartbeat() }
func (l *lateHost) Dispatch(c *command)   { l.m.Dispatch(c) }

// Run starts everything and blocks until ctx ends. Guests keep running
// across a return; only hostd's goroutines stop.
func Run(ctx context.Context, o Options, log *slog.Logger) error {
	st, err := state.Open(filepath.Join(o.StateDir, "state.db"))
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }() // read-only at exit; a close error changes nothing
	r := shell.Exec{}
	l := &lvm.Real{VG: o.VG, Pool: o.Pool, R: r}
	id, err := EnsureIdentity(ctx, o, log, r, l)
	if err != nil {
		return err
	}
	roots, err := register.LoadRoots(o.APICA)
	if err != nil {
		return err
	}
	m := metrics.New()
	total, _, _ := hostinfo.MemInfo() // an unreadable meminfo means no memory accounting; Defaults() copes with zero
	d := &Daemon{o: o, log: log, st: st, metrics: m, id: id, start: time.Now()}
	d.dialer = &rotatingDialer{inner: stream.GRPCDialer{Addr: o.APIAddr, TLS: id.TLSConfig(roots, o.APIServerName)}}
	lh := &lateHost{}
	d.strm = stream.New(stream.Config{}, streamDialer{d.dialer}, lh, m, log)

	var blob snapshot.Blob
	switch {
	case o.SnapshotDir != "":
		blob = &snapshot.FileBlob{Dir: o.SnapshotDir}
	case o.BlobURL != "":
		blob, err = snapshot.NewAzureBlob(o.BlobURL, o.BlobContainer, o.BlobIdentity)
		if err != nil {
			return err
		}
	default:
		return errors.New("hostd: set --snapshot-dir or --blob-url")
	}
	var dialer vsockclient.Dialer = vsockclient.CHDialer{}
	if o.GuestdUnix {
		dialer = vsockclient.UnixDialer{}
	}
	nix := (&nixbuild.Real{R: r, BuildsDir: o.BuildsDir, BaseDir: o.BaseDir, BaseRepoURL: o.BaseRepoURL, Roots: gcroot.Roots{Dir: o.GCRootsDir}, UseScope: true, Timeout: "timeout", Substituters: o.Substituters}).Defaults()
	cfg := guest.Config{
		HostID: id.Host.HostID, GuestsDir: o.GuestsDir, GuestCIDR: id.Host.GuestCIDR, TotalMemBytes: total,
		MaxOps: o.MaxOps, MaxBuilds: o.MaxBuilds, StoreExport: o.StoreExport, VirtiofsUser: o.VirtiofsUser,
	}
	if os.Getenv("REPOSE_HOSTD_TESTING") == "1" {
		cfg.FailAtStep = o.FailAtStep
	}
	consoles := &consoleSet{log: log}
	mgr, err := guest.New(cfg, guest.Deps{
		State: st, LVM: l, Net: hnet.NewReal(r), Systemd: systemd.NewReal(r), CH: &ch.HTTP{}, Guestd: dialer, Nix: nix,
		Roots: gcroot.Roots{Dir: o.GCRootsDir}, Blob: blob, Stream: &snapshot.Pipeline{R: r}, Emit: d.strm, Metrics: m, Log: log,
		MemInfo: hostinfo.MemInfo, Load1: hostinfo.Load1, StoreStat: hostinfo.StoreStat, ConsoleStart: consoles.start,
	})
	if err != nil {
		return err
	}
	lh.m = mgr
	d.mgr = mgr
	if err := mgr.Reconcile(ctx); err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}
	mgr.Run()
	defer mgr.Close()

	var wg sync.WaitGroup
	run := func(name string, f func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f()
		}()
		log.Info("started "+name, "component", "hostd", "event", "start", "part", name)
	}
	run("stream", func() { d.strm.Run(ctx) })
	run("samples", func() { d.samplesLoop(ctx) })
	run("rotation", func() { d.rotationLoop(ctx, roots) })
	run("prune", func() { d.pruneLoop(ctx) })
	run("metrics", func() { d.serveMetrics(ctx) })
	run("control", func() {
		if err := control.Serve(ctx, o.ControlSock, d); err != nil {
			log.Error("control socket failed", "component", "hostd", "event", "control", "err", err.Error())
		}
	})
	<-ctx.Done()
	wg.Wait()
	consoles.stopAll()
	return nil
}

func (d *Daemon) samplesLoop(ctx context.Context) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.strm.Samples(d.mgr.CollectSamples(ctx))
		}
	}
}

func (d *Daemon) rotationLoop(ctx context.Context, roots *x509.CertPool) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		d.idMu.Lock()
		cur := d.id
		d.idMu.Unlock()
		if !cur.ShouldRotate(time.Now()) {
			continue
		}
		cfg := register.Config{Dir: d.o.StateDir, APIAddr: d.o.APIAddr, ServerName: d.o.APIServerName, Roots: roots, Info: hostinfo.Collect(ctx, shell.Exec{}, nil)}
		next, err := register.Rotate(ctx, cfg, cur)
		if err != nil {
			d.log.Warn("certificate rotation failed", "component", "hostd", "event", "rotate", "err", err.Error())
			continue
		}
		d.idMu.Lock()
		d.id = next
		d.idMu.Unlock()
		d.dialer.mu.Lock()
		d.dialer.inner = stream.GRPCDialer{Addr: d.o.APIAddr, TLS: next.TLSConfig(roots, d.o.APIServerName)}
		d.dialer.mu.Unlock()
		d.log.Info("certificate rotated", "component", "hostd", "event", "rotate", "not_after", next.NotAfter.Format(time.RFC3339))
		d.strm.Reconnect()
	}
}

func (d *Daemon) pruneLoop(ctx context.Context) {
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		if n, err := d.st.PruneCommands(time.Now().Add(-7 * 24 * time.Hour)); err == nil && n > 0 {
			d.log.Info("pruned command results", "component", "hostd", "event", "prune", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (d *Daemon) serveMetrics(ctx context.Context) {
	addr := d.o.MetricsAddr
	if addr == "" {
		addr = "127.0.0.1:9101"
		if a := d.id.Host.WG.Address; a != "" {
			if ip, _, err := net.ParseCIDR(a); err == nil {
				addr = ip.String() + ":9101"
			}
		}
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", d.metrics.Handler())
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx) // shutting down
	}()
	d.log.Info("metrics listening", "component", "hostd", "event", "metrics", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		d.log.Error("metrics server failed", "component", "hostd", "event", "metrics", "err", err.Error())
	}
}

// --- control.Backend ----------------------------------------------------

func (d *Daemon) Status(ctx context.Context) (*control.StatusReply, error) {
	gs, err := d.mgr.Guests(ctx)
	if err != nil {
		return nil, err
	}
	by := map[string]int{}
	for _, g := range gs {
		by[g.State]++
	}
	return &control.StatusReply{
		HostID: d.id.Host.HostID, Version: d.o.Version, Connected: d.strm.Connected(), Draining: d.mgr.Draining(),
		Guests: by, FreeMem: d.mgr.FreeMemBytes(), PoolFree: d.mgr.PoolFreeBytes(), Uptime: time.Since(d.start).Round(time.Second).String(),
	}, nil
}

func (d *Daemon) Guests(ctx context.Context) ([]guest.Status, error) { return d.mgr.Guests(ctx) }

func (d *Daemon) SnapshotAll(reason string) ([]string, error) { return d.mgr.SnapshotAll(reason) }

func (d *Daemon) Drain(on bool) error {
	if on {
		return d.st.SetDraining(true)
	}
	return d.mgr.Undrain()
}

func (d *Daemon) Reconcile(ctx context.Context, rebuild bool) ([]string, error) {
	if rebuild {
		return d.mgr.Rebuild(ctx)
	}
	return nil, d.mgr.Reconcile(ctx)
}

func (d *Daemon) ExportState(w io.Writer) error { return d.st.Export(w) }

// --- console capture --------------------------------------------------

type consoleSet struct {
	log *slog.Logger
	mu  sync.Mutex
	run map[string]context.CancelFunc
}

func (c *consoleSet) start(guestID, dir string) func() {
	c.mu.Lock()
	if c.run == nil {
		c.run = map[string]context.CancelFunc{}
	}
	if cancel, ok := c.run[guestID]; ok {
		cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.run[guestID] = cancel
	c.mu.Unlock()
	t := console.New(ch.ConsoleSocket(dir), filepath.Join(dir, "console.log"))
	go func() {
		if err := t.Run(ctx); err != nil {
			c.log.Warn("console capture ended", "component", "hostd", "event", "console", "guest_id", guestID, "err", err.Error())
		}
	}()
	return func() {
		cancel()
		c.mu.Lock()
		delete(c.run, guestID)
		c.mu.Unlock()
	}
}

func (c *consoleSet) stopAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, cancel := range c.run {
		cancel()
	}
}

// AuditLogin writes the journald line the PAM hook produces for an
// operator login (the api-side audit row is workstream 05's).
func AuditLogin(log *slog.Logger, hostID string) {
	user := os.Getenv("PAM_USER")
	kind := os.Getenv("PAM_TYPE")
	if kind == "" {
		kind = "unknown"
	}
	log.Log(context.Background(), slog.Level(2), "operator login", "component", "hostd", "event", "operator_login", "host_id", hostID, "pam_type", kind, "user_present", user != "")
}
