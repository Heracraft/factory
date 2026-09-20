package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/heracraft/repose/internal/obs"
	obsmetrics "github.com/heracraft/repose/internal/obs/metrics"
)

// The messages a user sees, as banners, for each refusal
// (06-gateway-edge.md §5.2, §5.3 and §6; docs/interfaces/ssh-gateway.md).
const (
	MsgCertRequired   = "permission denied (certificate required)"
	MsgBadCA          = "permission denied (certificate not signed by the repose CA)"
	MsgExpired        = "permission denied (certificate expired)"
	MsgNotYetValid    = "permission denied (certificate not yet valid)"
	MsgRevoked        = "permission denied (certificate revoked)"
	MsgWrongPrincipal = "certificate not valid for this project"
	MsgControlPlane   = "gateway cannot reach control plane; try again shortly"
	MsgBusy           = "gateway busy"
	MsgRateLimited    = "too many authentication attempts from your address; try again later"
	MsgNotReady       = "environment is not accepting connections yet"
	// MsgStoppedFmt takes the slug.
	MsgStoppedFmt = "%s is stopped; run `repose start`"
	// MsgNotFoundFmt takes the login name.
	MsgNotFoundFmt = "no such project: %s"
	// MsgStateFmt takes the slug and a state other than running or stopped.
	MsgStateFmt = "%s is %s; " + MsgNotReady
)

// Config configures a Gateway.
type Config struct {
	API *Client
	// HostKey is presented to clients: a cert signer when the Host CA
	// certificate exists, the plain key otherwise.
	HostKey ssh.Signer
	// GatewayKey is the gateway's own key pair, certified per project by
	// the api for the dial to the guest.
	GatewayKey ssh.Signer
	// GuestPort is the guest sshd port (22).
	GuestPort int

	MaxConns     int
	MaxAuthPerIP int
	AuthTimeout  time.Duration
	MaxAuthTries int
	DialTimeout  time.Duration
	Keepalive    time.Duration
	SessionCap   time.Duration
	// RevocationRefresh and CARefresh override the §5.2 windows (tests).
	RevocationRefresh time.Duration
	CARefresh         time.Duration

	Log     *slog.Logger
	Metrics *obsmetrics.GatewayMetrics
	Clock   func() time.Time
	// Dial replaces the TCP dial to the guest (tests).
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
	// ServerVersion is the SSH version string; empty means the default.
	ServerVersion string
}

// Gateway is one running gateway.
type Gateway struct {
	cfg     Config
	revoked *revocationCache
	cas     *caCache
	routes  *routeCache
	certs   *certCache
	limiter *limiter
	conns   connCounter
	wg      sync.WaitGroup
	log     *slog.Logger
}

// New validates the configuration and applies the defaults of §5.2.
func New(cfg Config) (*Gateway, error) {
	if cfg.API == nil {
		return nil, errors.New("gateway: api client is required")
	}
	if cfg.HostKey == nil || cfg.GatewayKey == nil {
		return nil, errors.New("gateway: host key and gateway key are required")
	}
	if cfg.GuestPort == 0 {
		cfg.GuestPort = 22
	}
	if cfg.MaxConns == 0 {
		cfg.MaxConns = DefaultMaxConns
	}
	if cfg.MaxAuthPerIP == 0 {
		cfg.MaxAuthPerIP = DefaultMaxAuthPerIP
	}
	if cfg.AuthTimeout == 0 {
		cfg.AuthTimeout = DefaultAuthTimeout
	}
	if cfg.MaxAuthTries == 0 {
		cfg.MaxAuthTries = DefaultMaxAuthTries
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 10 * time.Second
	}
	if cfg.Keepalive == 0 {
		cfg.Keepalive = 30 * time.Second
	}
	if cfg.SessionCap == 0 {
		cfg.SessionCap = 24 * time.Hour
	}
	if cfg.RevocationRefresh == 0 {
		cfg.RevocationRefresh = RevocationRefresh
	}
	if cfg.CARefresh == 0 {
		cfg.CARefresh = CARefresh
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.DiscardHandler)
	}
	if cfg.Metrics == nil {
		cfg.Metrics = obsmetrics.NewGatewayMetrics(obsmetrics.New(obs.ComponentGateway))
	}
	if cfg.Dial == nil {
		cfg.Dial = (&net.Dialer{}).DialContext
	}
	g := &Gateway{
		cfg:     cfg,
		revoked: newRevocationCache(cfg.Clock),
		cas:     newCACache(cfg.Clock),
		routes:  newRouteCache(cfg.Clock),
		certs:   newCertCache(cfg.Clock),
		limiter: newLimiter(cfg.MaxAuthPerIP, cfg.Clock),
		conns:   connCounter{max: cfg.MaxConns},
		log:     cfg.Log,
	}
	return g, nil
}

// Prime fetches the CA keys and the revocation list once. Serve works
// without it (every connection is refused with MsgControlPlane until a
// refresh succeeds), so a start with the api down still binds the port.
func (g *Gateway) Prime(ctx context.Context) error {
	if err := g.cas.Refresh(ctx, g.cfg.API); err != nil {
		return fmt.Errorf("ca: %w", err)
	}
	if err := g.revoked.Refresh(ctx, g.cfg.API); err != nil {
		return fmt.Errorf("revoked: %w", err)
	}
	return nil
}

// SetCAs installs CA keys without the api (tests, and a hostdev edge).
func (g *Gateway) SetCAs(userLine, hostLine string) error { return g.cas.Set(userLine, hostLine) }

// Revoke pushes a serial into the revocation set immediately.
func (g *Gateway) Revoke(serial uint64) { g.revoked.Push(serial) }

// RefreshLoop keeps the revocation list (every 30 s) and the CA keys
// (hourly) fresh until ctx ends. Failures are logged and counted in the
// cache age; auth refuses once the age passes StaleAfter.
func (g *Gateway) RefreshLoop(ctx context.Context) {
	revTick := time.NewTicker(g.cfg.RevocationRefresh)
	caTick := time.NewTicker(g.cfg.CARefresh)
	defer revTick.Stop()
	defer caTick.Stop()
	refreshRev := func() {
		rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := g.revoked.Refresh(rctx, g.cfg.API); err != nil {
			g.log.Warn("revocation refresh failed", "event", "route_fail", "reason", "revoked_refresh", "age_ms", g.revoked.Age().Milliseconds(), "err", err.Error())
		}
		g.cfg.Metrics.RevocationCacheAge.Set(g.revoked.Age().Seconds())
	}
	refreshCA := func() {
		rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := g.cas.Refresh(rctx, g.cfg.API); err != nil {
			g.log.Warn("ca refresh failed", "event", "route_fail", "reason", "ca_refresh", "age_ms", g.cas.Age().Milliseconds(), "err", err.Error())
		}
	}
	if g.cas.Age() > g.cfg.CARefresh {
		refreshCA()
	}
	if g.revoked.Age() > g.cfg.RevocationRefresh {
		refreshRev()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-revTick.C:
			refreshRev()
			// A CA that never loaded is retried with the revocations.
			if g.cas.Age() > g.cfg.CARefresh {
				refreshCA()
			}
		case <-caTick.C:
			refreshCA()
		}
	}
}

// Serve accepts connections until the listener closes or ctx ends, then
// waits for open relays to finish (they end when ctx ends).
func (g *Gateway) Serve(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = ln.Close() // unblocks Accept; the error is the listener's, not ours
	}()
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				g.wg.Wait()
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			g.wg.Wait()
			return err
		}
		g.wg.Add(1)
		go func() {
			defer g.wg.Done()
			g.HandleConn(ctx, c)
		}()
	}
}

// Open is the number of relays open.
func (g *Gateway) Open() int { return g.conns.open() }

// connState is what authentication decides for one connection.
type connState struct {
	busy    bool
	limited bool
	// set by the public key callback on success
	route  *Route
	slug   string
	handle string
	serial uint64
	keyID  string
	// the last failure, for the log line
	result string
}

// HandleConn runs one connection: limits, SSH handshake with certificate
// authentication, then the relay.
func (g *Gateway) HandleConn(ctx context.Context, c net.Conn) {
	src := sourceKey(c.RemoteAddr())
	prefix := sourcePrefix(c.RemoteAddr())
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetKeepAlive(true)                  // best effort; the SSH keepalive is the real check
		_ = tc.SetKeepAlivePeriod(g.cfg.Keepalive) // same
	}
	st := &connState{}
	if !g.conns.acquire() {
		st.busy = true
	} else {
		defer g.conns.release()
	}
	tracked := false
	if !st.busy && !g.limiter.beginAuth(src) {
		st.limited = true
	} else if !st.busy {
		tracked = true
	}

	_ = c.SetDeadline(g.cfg.Clock().Add(g.cfg.AuthTimeout)) // a conn that cannot take a deadline fails the handshake instead
	sconn, chans, reqs, err := ssh.NewServerConn(c, g.serverConfig(ctx, st))
	// The per-source slot covers the authentication phase only (§5.2: N
	// concurrent auth *attempts*), not the whole relay; release it here so a
	// user's own many connections are not throttled against each other.
	if tracked {
		g.limiter.endAuth(src, err != nil)
	}
	if err != nil {
		_ = c.Close() // already failing; nothing to report to
		if st.result == "" {
			st.result = "handshake"
		}
		if st.result != ResultNoCert || g.log.Enabled(ctx, slog.LevelDebug) {
			g.log.Info("authentication failed", "event", "auth_fail", "reason", st.result, "source_prefix", prefix)
		}
		return
	}
	_ = c.SetDeadline(time.Time{}) // same conn as above; a failure here surfaces on the next read
	sess := &session{
		gw:        g,
		conn:      sconn,
		chans:     chans,
		reqs:      reqs,
		route:     st.route,
		slug:      st.slug,
		handle:    st.handle,
		serial:    st.serial,
		prefix:    prefix,
		startedAt: g.cfg.Clock(),
	}
	sess.run(ctx)
}

// serverConfig is the per-connection SSH server configuration.
func (g *Gateway) serverConfig(ctx context.Context, st *connState) *ssh.ServerConfig {
	cfg := &ssh.ServerConfig{
		MaxAuthTries:  g.cfg.MaxAuthTries,
		ServerVersion: g.cfg.ServerVersion,
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			return g.authenticate(ctx, st, conn, key)
		},
		BannerCallback: func(conn ssh.ConnMetadata) string {
			// The login name arrives with the first auth request; a bad one
			// is told before any key is tried.
			if _, _, err := ParseLogin(conn.User()); err != nil {
				return BadLoginMessage
			}
			return ""
		},
	}
	cfg.AddHostKey(g.cfg.HostKey)
	return cfg
}

// fail records the result and returns the banner error.
func (g *Gateway) fail(st *connState, result, message string) (*ssh.Permissions, error) {
	st.result = result
	g.cfg.Metrics.AuthFailTotal.WithLabelValues(result).Inc()
	return nil, &ssh.BannerError{Err: errors.New(result), Message: message}
}

// authenticate is the decision chain of §5.2: certificate, CA, validity,
// revocation, route, principal, state.
func (g *Gateway) authenticate(ctx context.Context, st *connState, conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	if st.busy {
		return g.fail(st, ResultBusy, MsgBusy)
	}
	if st.limited {
		return g.fail(st, ResultRateLimited, MsgRateLimited)
	}
	slug, handle, err := ParseLogin(conn.User())
	if err != nil {
		return g.fail(st, ResultBadLogin, BadLoginMessage)
	}
	cert, ok := key.(*ssh.Certificate)
	if !ok || cert.CertType != ssh.UserCert {
		return g.fail(st, ResultNoCert, MsgCertRequired)
	}
	userCA, _, err := g.cas.Keys()
	if err != nil {
		return g.fail(st, ResultRouteError, MsgControlPlane)
	}
	if g.revoked.Age() > StaleAfter {
		return g.fail(st, ResultRouteError, MsgControlPlane)
	}
	if !keysEqual(cert.SignatureKey, userCA) {
		return g.fail(st, ResultBadCA, MsgBadCA)
	}
	now := g.cfg.Clock().Unix()
	if now < int64(cert.ValidAfter) {
		return g.fail(st, ResultExpired, MsgNotYetValid)
	}
	if cert.ValidBefore != uint64(ssh.CertTimeInfinity) && now >= int64(cert.ValidBefore) {
		return g.fail(st, ResultExpired, MsgExpired)
	}
	if g.revoked.IsRevoked(cert.Serial) {
		return g.fail(st, ResultRevoked, MsgRevoked)
	}
	login := slug + "." + handle
	started := g.cfg.Clock()
	route, err := g.routes.Lookup(ctx, g.cfg.API, login)
	g.cfg.Metrics.RouteDuration.Observe(g.cfg.Clock().Sub(started).Seconds())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return g.fail(st, ResultNotFound, fmt.Sprintf(MsgNotFoundFmt, login))
		}
		g.log.Warn("route lookup failed", "event", "route_fail", "reason", "api", "err", err.Error())
		return g.fail(st, ResultRouteError, MsgControlPlane)
	}
	if len(cert.ValidPrincipals) == 0 || !contains(cert.ValidPrincipals, route.ProjectID) {
		return g.fail(st, ResultWrongPrincipal, MsgWrongPrincipal)
	}
	switch route.State {
	case "running":
		if route.GuestIP == "" || route.HostUnreachable {
			return g.fail(st, ResultRouteError, fmt.Sprintf(MsgStateFmt, slug, "on an unreachable host"))
		}
	case "stopped", "stopping":
		return g.fail(st, ResultStopped, fmt.Sprintf(MsgStoppedFmt, slug))
	default:
		return g.fail(st, ResultStopped, fmt.Sprintf(MsgStateFmt, slug, route.State))
	}
	// The authoritative check: signature, critical options, validity and
	// revocation again, with the project id as the principal.
	checker := &ssh.CertChecker{
		SupportedCriticalOptions: []string{"source-address"},
		IsUserAuthority:          func(auth ssh.PublicKey) bool { return keysEqual(auth, userCA) },
		IsRevoked:                func(c *ssh.Certificate) bool { return g.revoked.IsRevoked(c.Serial) },
		Clock:                    g.cfg.Clock,
	}
	if err := checker.CheckCert(route.ProjectID, cert); err != nil {
		return g.fail(st, ResultBadCA, MsgBadCA)
	}
	st.route, st.slug, st.handle, st.serial, st.keyID, st.result = route, slug, handle, cert.Serial, cert.KeyId, ResultOK
	perms := &ssh.Permissions{
		CriticalOptions: cert.CriticalOptions,
		Extensions: map[string]string{
			"repose-project-id":  route.ProjectID,
			"repose-guest-ip":    route.GuestIP,
			"repose-cert-serial": strconv.FormatUint(cert.Serial, 10),
		},
	}
	return perms, nil
}

func keysEqual(a, b ssh.PublicKey) bool {
	return a != nil && b != nil && a.Type() == b.Type() && bytes.Equal(a.Marshal(), b.Marshal())
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// sourcePrefix is the source address truncated to /24 (IPv4) or /48
// (IPv6), the only form of it that is logged (§5.5).
func sourcePrefix(addr net.Addr) string {
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		host = addr.String()
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "unknown"
	}
	if v4 := ip.To4(); v4 != nil {
		return (&net.IPNet{IP: v4.Mask(net.CIDRMask(24, 32)), Mask: net.CIDRMask(24, 32)}).String()
	}
	return (&net.IPNet{IP: ip.Mask(net.CIDRMask(48, 128)), Mask: net.CIDRMask(48, 128)}).String()
}
