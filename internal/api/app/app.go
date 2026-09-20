package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/heracraft/repose/internal/api/auth"
	"github.com/heracraft/repose/internal/api/basebump"
	"github.com/heracraft/repose/internal/api/buildlog"
	"github.com/heracraft/repose/internal/api/ca"
	"github.com/heracraft/repose/internal/api/config"
	"github.com/heracraft/repose/internal/api/events"
	"github.com/heracraft/repose/internal/api/hostmgr"
	httpapi "github.com/heracraft/repose/internal/api/http"
	"github.com/heracraft/repose/internal/api/meter"
	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/api/notify"
	"github.com/heracraft/repose/internal/api/ops"
	"github.com/heracraft/repose/internal/api/pki"
	"github.com/heracraft/repose/internal/api/secrets"
	"github.com/heracraft/repose/internal/api/snapshots"
	"github.com/heracraft/repose/internal/billing"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/fakes/kv"
	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
	"github.com/heracraft/repose/internal/obs"
	"github.com/heracraft/repose/internal/obs/instrument"
	obsmetrics "github.com/heracraft/repose/internal/obs/metrics"

	"github.com/google/uuid"
)

// App is the assembled process.
type App struct {
	cfg     Config
	log     *slog.Logger
	pool    *db.Pool
	reg     *prometheus.Registry
	m       *metrics.M
	sec     *secrets.Store
	ca      *ca.CA
	hostMgr *hostmgr.Server
	engine  *ops.Engine
	logs    *buildlog.Store
	events  *events.Ingest
	meterIn *meter.Ingest
	outbox  *notify.Outbox
	stripe  *billing.Stripe
	hooks   *billing.Webhooks
	bcfg    billing.Config
	server  *httpapi.Server
	version string
	otelOff func(context.Context) error
}

// New wires the process. Nothing listens yet.
func New(ctx context.Context, cfg Config, version string) (*App, error) {
	level := slog.LevelInfo
	if lv, err := obs.ParseLevel(os.Getenv("LOG_LEVEL")); err == nil {
		level = lv
	}
	log := obs.NewLogger(obs.LogOptions{Component: obs.ComponentAPI, Level: level})
	a := &App{cfg: cfg, log: log, version: version}
	// One tracing setup for every binary (DECISIONS I-59): no exporter and no
	// connection when OTEL_EXPORTER_OTLP_ENDPOINT is unset, OTLP over HTTP or
	// gRPC as OTEL_EXPORTER_OTLP_PROTOCOL asks.
	_, off, err := instrument.SetupTracing(ctx, instrument.TraceOptions{
		Component: obs.ComponentAPI, Version: version, Insecure: true,
	})
	if err != nil {
		return nil, err
	}
	a.otelOff = off
	enabled := instrument.TracingEnabled()
	log.Info("starting", "event", "start", "mode", cfg.Mode, "version", version, "otel", enabled, "dev", cfg.Dev)
	a.pool, err = db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if cfg.Migrate {
		applied, err := db.MigrateUp(ctx, a.pool)
		if err != nil {
			return nil, fmt.Errorf("migrate: %w", err)
		}
		log.Info("migrations applied", "event", "migrate", "count", len(applied))
	}
	st, err := db.MigrateStatus(ctx, a.pool)
	if err != nil {
		return nil, err
	}
	if len(st.Pending) > 0 {
		log.Warn("migrations pending; /healthz reports unhealthy until `repose-admin db migrate` runs", "event", "migrate_pending", "count", len(st.Pending))
	} else if err := db.EnsurePartitions(ctx, a.pool, time.Now()); err != nil {
		return nil, err
	}
	// The registry from internal/obs/metrics refuses a metric outside the
	// repose_ namespace or with a label outside the low-cardinality list, so
	// every series in internal/api/metrics is checked at startup
	// (docs/workstreams/10-observability.md §5).
	om := obsmetrics.NewVersion(obs.ComponentAPI, version)
	a.reg = om.Registry()
	a.m = metrics.New(om)
	var kvs secrets.KeyVault
	if cfg.KeyVaultURL != "" {
		azkv, err := secrets.NewAzureKV(cfg.KeyVaultURL, cfg.KeyVaultKeyName, nil)
		if err != nil {
			return nil, err
		}
		kvs = azkv
	} else {
		log.Warn("REPOSE_DEV=1: using the in-memory key vault; secrets do not survive a restart", "event", "dev_kv")
		kvs = kv.New()
	}
	a.sec = secrets.New(a.pool, kvs)
	a.ca, err = ca.Load(ctx, a.pool, a.sec)
	if errors.Is(err, ca.ErrNotInitialised) && cfg.Dev {
		log.Warn("REPOSE_DEV=1: initialising the CA in the in-memory key vault", "event", "dev_ca")
		if err := ca.Init(ctx, a.sec); err != nil {
			return nil, err
		}
		a.ca, err = ca.Load(ctx, a.pool, a.sec)
	}
	if err != nil {
		return nil, err
	}
	a.logs = buildlog.New(a.pool, log)
	a.events = events.New(a.pool, a.m, log)
	a.meterIn = meter.New(a.pool, a.m, log)
	a.hostMgr = hostmgr.New(a.pool, a.ca.X509(), cfg.ReplicaID, a.m, log)
	a.engine = ops.New(a.pool, a.hostMgr, a.ca, a.sec, a.logs, a.events, a.m, log, ops.Config{BaseRef: cfg.BaseRef})
	a.hostMgr.SetHandlers(hostmgr.Handlers{
		Hello:   a.engine.OnHello,
		Result:  a.engine.OnResult,
		Samples: a.meterIn.OnSamples,
		Event:   a.events.OnEvent,
		BuildLog: func(ctx context.Context, hostID uuid.UUID, l *hostdv1.BuildLog) {
			if opID, ok := a.logs.OpFor(l.CommandId); ok {
				a.logs.Append(opID, int64(l.Seq), l.Line)
			}
		},
	})
	senders := map[string]notify.Sender{"email": &notify.Email{APIKey: cfg.ResendAPIKey, From: cfg.NotifyFrom}, "ntfy": &notify.Ntfy{}}
	a.outbox = notify.New(a.pool, senders, a.m, log)
	a.outbox.Dashboard = cfg.DashboardURL
	a.outbox.APIBase = cfg.APIResource
	unsub, err := notify.LoadOrCreateUnsubscriber(ctx, a.sec)
	if err != nil {
		log.Warn("unsubscribe key unavailable; email unsubscribe links are disabled", "event", "notify_unsub_unavailable", "err", err.Error())
	} else {
		a.outbox.Unsub = unsub
	}
	// Billing (workstream 09). With no STRIPE_SECRET_KEY the api starts
	// normally and the billing routes answer 503 billing_disabled
	// (DECISIONS I-16); with one, a half-configured Stripe is refused
	// rather than silently billing nothing.
	bcfg, stripeOn := billing.ConfigFromEnv()
	a.bcfg = bcfg
	var portal billing.Portal = billing.DisabledPortal{}
	if stripeOn {
		st, err := billing.NewStripe(bcfg, a.pool, log)
		if err != nil {
			return nil, fmt.Errorf("billing: %w", err)
		}
		a.stripe = st
		portal = st
		a.hooks = billing.NewWebhooks(a.pool, bcfg.WebhookSecret, log)
		a.hooks.OnCardAttached = st.OnCardAttached
		log.Info("billing enabled", "event", "billing_enabled", "enforced", bcfg.Enforce, "automatic_tax", bcfg.AutomaticTax)
	} else {
		log.Info("billing disabled until STRIPE_SECRET_KEY is set (DECISIONS I-16)", "event", "billing_disabled")
	}
	if _, err := billing.RecordEnforcement(ctx, a.pool, bcfg.Enforce, "api", log); err != nil {
		return nil, err
	}

	parser, ok := config.NewParser()
	if !ok {
		log.Warn("nix-instantiate not found; fragments are accepted without a parse check", "event", "config_parse_unavailable")
		parser = nil
	}
	var verifier *auth.Verifier
	var users *auth.Provisioner
	if cfg.LogtoIssuer != "" {
		verifier = auth.NewVerifier(cfg.LogtoIssuer, cfg.APIResource, nil)
		users = auth.NewProvisioner(a.pool, auth.NewLogtoManagement(cfg.LogtoIssuer, cfg.LogtoM2MID, cfg.LogtoM2MSecret, nil))
	}
	a.server = httpapi.New(httpapi.Deps{
		Pool: a.pool, Verifier: verifier, Users: users, CA: a.ca, Secrets: a.sec, Engine: a.engine, Logs: a.logs, Events: a.events, Outbox: a.outbox, Unsub: unsub,
		Parser: parser, Metrics: a.m, Registry: a.reg, Log: log, Billing: portal, Webhooks: a.hooks, BillingEnforce: bcfg.Enforce,
		Gateway: httpapi.Gateway{Host: cfg.GatewayHost, Port: cfg.GatewayPort},
		Migrations: func(ctx context.Context) (int, error) {
			st, err := db.MigrateStatus(ctx, a.pool)
			return len(st.Pending), err
		},
	})
	return a, nil
}

// HostCA exposes the X.509 host authority (tests, repose-admin).
func (a *App) HostCA() *pki.CA { return a.ca.X509() }

// Run serves until ctx ends, then drains.
func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 8)
	var httpSrv, internalSrv, metricsSrv *http.Server
	var gs interface {
		GracefulStop()
		Stop()
	}
	bg, cancelBG := context.WithCancel(ctx)
	defer cancelBG()
	go a.logs.Run(bg)

	if a.cfg.Mode == "http" || a.cfg.Mode == "all" {
		if a.cfg.LogtoIssuer == "" {
			a.log.Warn("REPOSE_DEV=1 without LOGTO_ISSUER: user routes will refuse every token", "event", "dev_no_logto")
		}
		httpSrv = &http.Server{Addr: a.cfg.Listen, Handler: a.server.Handler(), ReadHeaderTimeout: 10 * time.Second}
		go func() { errCh <- serve(httpSrv, "http", a.log) }()
	}
	if a.cfg.Mode == "grpc" || a.cfg.Mode == "all" {
		var serverCert *tls.Certificate
		if a.cfg.GRPCServerCert != "" {
			c, err := tls.LoadX509KeyPair(a.cfg.GRPCServerCert, a.cfg.GRPCServerKey)
			if err != nil {
				return fmt.Errorf("grpc server certificate: %w", err)
			}
			serverCert = &c
		}
		names := a.cfg.GRPCServerNames
		if len(names) == 0 {
			names = []string{"localhost", "127.0.0.1"}
		}
		tlsCfg, err := a.hostMgr.TLSConfig(serverCert, names...)
		if err != nil {
			return err
		}
		grpcSrv := a.hostMgr.GRPCServer(tlsCfg)
		gs = grpcSrv
		ln, err := net.Listen("tcp", a.cfg.GRPCListen)
		if err != nil {
			return fmt.Errorf("grpc listen: %w", err)
		}
		a.log.Info("grpc listening", "event", "listen", "addr", a.cfg.GRPCListen)
		go func() { errCh <- grpcSrv.Serve(ln) }()
		// /internal over HTTPS with the gateway's client certificate.
		itls := tlsCfg.Clone()
		itls.ClientAuth = tls.RequireAndVerifyClientCert
		internalSrv = &http.Server{Addr: a.cfg.InternalListen, Handler: a.server.InternalHandler(), TLSConfig: itls, ReadHeaderTimeout: 10 * time.Second}
		go func() {
			a.log.Info("internal listening", "event", "listen", "addr", a.cfg.InternalListen)
			err := internalSrv.ListenAndServeTLS("", "")
			if !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("internal: %w", err)
			}
		}()
		go a.engine.Run(bg)
		go a.outbox.Run(bg)
		go a.loops(bg)
	}
	if a.cfg.MetricsListen != "" && a.cfg.MetricsListen != "off" {
		metricsSrv = &http.Server{Addr: a.cfg.MetricsListen, Handler: a.server.MetricsHandler(), ReadHeaderTimeout: 10 * time.Second}
		go func() { errCh <- serve(metricsSrv, "metrics", a.log) }()
	}
	a.server.SetReady(true)
	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errCh:
		if runErr != nil {
			a.log.Error("listener failed", "event", "listen_fail", "err", runErr.Error())
		}
	}
	a.log.Info("shutting down", "event", "shutdown")
	a.server.SetReady(false)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if httpSrv != nil {
		_ = httpSrv.Shutdown(shutdownCtx) // best effort during drain
	}
	if internalSrv != nil {
		_ = internalSrv.Shutdown(shutdownCtx)
	}
	if metricsSrv != nil {
		_ = metricsSrv.Shutdown(shutdownCtx)
	}
	if gs != nil {
		done := make(chan struct{})
		go func() { gs.GracefulStop(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			gs.Stop()
		}
	}
	cancelBG()
	if a.otelOff != nil {
		_ = a.otelOff(shutdownCtx)
	}
	a.pool.Close()
	return runErr
}

func serve(s *http.Server, name string, log *slog.Logger) error {
	log.Info(name+" listening", "event", "listen", "addr", s.Addr)
	err := s.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("%s: %w", name, err)
}

// loops runs the periodic jobs of the grpc app.
func (a *App) loops(ctx context.Context) {
	var blob snapshots.BlobStore
	if a.cfg.BlobAccountURL != "" {
		b, err := snapshots.NewAzureBlob(a.cfg.BlobAccountURL, a.cfg.BlobContainer, nil)
		if err != nil {
			a.log.Error("blob store", "event", "blob_fail", "err", err.Error())
		} else {
			blob = b
		}
	}
	if blob == nil {
		a.log.Warn("BLOB_ACCOUNT_URL unset: snapshot expiry only marks rows", "event", "blob_unset")
		blob = markOnly{}
	}
	expiry := snapshots.New(a.pool, blob, a.m, a.log)
	go expiry.Run(ctx, 24*time.Hour)
	var pusher billing.UsagePusher = billing.Disabled{}
	var reader billing.Reader
	if a.stripe != nil {
		pusher = a.stripe
		reader = a.stripe
	}
	rollup := billing.NewRollup(a.pool, pusher, a.m, a.log)
	dunning := billing.NewDunning(a.pool, a.engine, a.events, a.log, a.bcfg.Enforce)
	reconciler := billing.NewReconciler(a.pool, reader, a.m, a.log)
	bump := basebump.New(a.pool, a.engine, a.events, a.log)
	a.engine.SetOnFinished(bump.OnOpFinished)
	go bump.Run(ctx)
	sweep := time.NewTicker(15 * time.Second)
	hourly := time.NewTicker(time.Minute)
	daily := time.NewTicker(24 * time.Hour)
	limiters := time.NewTicker(10 * time.Minute)
	defer sweep.Stop()
	defer hourly.Stop()
	defer daily.Stop()
	defer limiters.Stop()
	lastRollup := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-sweep.C:
			if _, err := a.hostMgr.Sweep(ctx); err != nil && ctx.Err() == nil {
				a.log.Error("host sweep", "event", "sweep_fail", "err", err.Error())
			}
		case now := <-hourly.C:
			// Rollup at :05 past each hour, under the advisory lock.
			if now.Minute() < 5 || now.Sub(lastRollup) < 50*time.Minute {
				continue
			}
			release, ok, err := db.TryLock(ctx, a.pool, db.LockRollup)
			if err != nil || !ok {
				if !ok {
					a.log.Info("rollup: not leader", "event", "rollup_skip")
				}
				continue
			}
			if _, err := rollup.Due(ctx); err != nil && ctx.Err() == nil {
				a.log.Error("rollup", "event", "rollup_fail", "err", err.Error())
			}
			// Past-due accounts are stopped from the same hourly tick and
			// under the same lock, so only one replica acts (§5.6).
			if _, err := dunning.Run(ctx); err != nil && ctx.Err() == nil {
				a.log.Error("dunning", "event", "dunning_fail", "err", err.Error())
			}
			release()
			lastRollup = now
		case <-daily.C:
			release, ok, err := db.TryLock(ctx, a.pool, db.LockCertCleanup)
			if err != nil || !ok {
				continue
			}
			if n, err := a.ca.Prune(ctx, 30*24*time.Hour); err == nil {
				a.log.Info("certificates pruned", "event", "cert_prune", "count", n)
			}
			if err := db.EnsurePartitions(ctx, a.pool, time.Now()); err != nil {
				a.m.PartitionDropFailTotal.Inc()
				a.log.Error("partitions", "event", "partition_fail", "err", err.Error())
			}
			if dropped, err := db.DropExpiredPartitions(ctx, a.pool, time.Now()); err != nil {
				a.m.PartitionDropFailTotal.Inc()
				a.log.Error("partition drop", "event", "partition_drop_fail", "err", err.Error())
			} else if len(dropped) > 0 {
				a.log.Info("partitions dropped", "event", "partition_drop", "count", len(dropped))
			}
			if n, err := a.logs.Trim(ctx, 20); err == nil && n > 0 {
				a.log.Info("build logs trimmed", "event", "buildlog_trim", "rows", n)
			}
			// The nightly reconciliation for the current period (§5.7). It
			// reports and never fixes; `repose-admin billing reconcile
			// --month` is the same comparison for a closed period.
			if ms, err := reconciler.Reconcile(ctx, time.Now()); errors.Is(err, billing.ErrNoReader) {
				a.log.Info("reconciliation skipped: Stripe is not readable", "event", "reconcile_skip")
			} else if err != nil {
				a.log.Error("reconciliation", "event", "reconcile_fail", "err", err.Error())
			} else if len(ms) > 0 {
				a.log.Error("reconciliation found differences", "event", "reconcile_mismatch", "users", len(ms))
			}
			release()
		case <-limiters.C:
			a.server.SweepLimiters()
		}
	}
}

// markOnly is the blob store when no account is configured: rows are
// marked deleted and the Blob lifecycle rules (workstream 11) reclaim.
type markOnly struct{}

func (markOnly) Delete(context.Context, string) error { return nil }
