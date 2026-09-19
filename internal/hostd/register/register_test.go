package register

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
	"github.com/heracraft/repose/internal/hostd/shell"
	"github.com/heracraft/repose/internal/hostd/testca"
)

type apiStub struct {
	hostdv1.UnimplementedHostServiceServer
	ca       *testca.CA
	used     map[string]bool
	validity time.Duration
	rotated  int
}

func (a *apiStub) Register(_ context.Context, req *hostdv1.RegisterRequest) (*hostdv1.RegisterResponse, error) {
	if req.JoinToken != "tok-1" || a.used[req.JoinToken] {
		return nil, status.Error(codes.PermissionDenied, "join token already used")
	}
	a.used[req.JoinToken] = true
	cert, key, err := a.ca.IssueClient("host-1", a.validity)
	if err != nil {
		return nil, err
	}
	return &hostdv1.RegisterResponse{HostId: "host-1", ClientCert: cert, ClientKey: key, GuestCidr: "10.64.4.0/22",
		WgPrivateKey: "wgpriv", Edge: &hostdv1.WireguardPeer{Endpoint: "edge:51820", PublicKey: "edgepub", Address: "10.255.0.7/16", AllowedIps: []string{"10.255.0.0/16"}}}, nil
}

func (a *apiStub) Rotate(_ context.Context, _ *hostdv1.RegisterRequest) (*hostdv1.RegisterResponse, error) {
	a.rotated++
	cert, key, err := a.ca.IssueClient("host-1", 30*24*time.Hour)
	if err != nil {
		return nil, err
	}
	return &hostdv1.RegisterResponse{HostId: "host-1", ClientCert: cert, ClientKey: key, GuestCidr: "10.64.4.0/22"}, nil
}

func TestRegisterThenRotate(t *testing.T) {
	ca, err := testca.New()
	if err != nil {
		t.Fatal(err)
	}
	srvCert, err := ca.ServerTLS("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	stub := &apiStub{ca: ca, used: map[string]bool{}, validity: 6 * 24 * time.Hour}
	gs := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{srvCert}, ClientAuth: tls.VerifyClientCertIfGiven, ClientCAs: ca.Pool(), MinVersion: tls.VersionTLS13})))
	hostdv1.RegisterHostServiceServer(gs, stub)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = gs.Serve(ln) }()
	defer gs.Stop()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "join-token")
	_ = os.WriteFile(tokenPath, []byte("tok-1\n"), 0o600)
	r := &shell.Fake{}
	cfg := Config{Dir: filepath.Join(dir, "hostd"), TokenPath: tokenPath, APIAddr: ln.Addr().String(), ServerName: "127.0.0.1", Roots: ca.Pool(), Info: &hostdv1.HostInfo{Hostname: "h"}, Runner: r, WGUnit: "wg-quick-wg0.service"}
	if _, err := Load(cfg.Dir); !os.IsNotExist(err) {
		t.Fatalf("expected no identity yet, got %v", err)
	}
	id, err := Register(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if id.Host.HostID != "host-1" || id.Host.GuestCIDR != "10.64.4.0/22" || id.Host.WG.EdgeEndpoint != "edge:51820" {
		t.Fatalf("host.json %+v", id.Host)
	}
	for _, f := range []string{CertFile, KeyFile, HostFile, WGFile} {
		st, err := os.Stat(filepath.Join(cfg.Dir, f))
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", f, st, err)
		}
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Fatal("join token not deleted")
	}
	if len(r.CallsWithPrefix("systemctl", "restart", "wg-quick-wg0.service")) != 1 {
		t.Fatalf("wg-quick not restarted: %v", r.Calls)
	}
	wg, _ := os.ReadFile(filepath.Join(cfg.Dir, WGFile))
	if string(wg) != "[Interface]\nPrivateKey = wgpriv\nAddress = 10.255.0.7/16\n\n[Peer]\nPublicKey = edgepub\nEndpoint = edge:51820\nAllowedIPs = 10.255.0.0/16\nPersistentKeepalive = 25\n" {
		t.Fatalf("wg0.conf:\n%s", wg)
	}
	loaded, err := Load(cfg.Dir)
	if err != nil || loaded.Host.HostID != "host-1" {
		t.Fatalf("load: %v %v", loaded, err)
	}
	// A 6-day certificate is inside the 5-day rotation window from day 2.
	if loaded.ShouldRotate(time.Now()) {
		t.Fatal("6-day cert should not rotate on day 0")
	}
	if !loaded.ShouldRotate(time.Now().Add(2 * 24 * time.Hour)) {
		t.Fatal("6-day cert should rotate on day 2")
	}
	rotated, err := Rotate(context.Background(), cfg, loaded)
	if err != nil {
		t.Fatal(err)
	}
	if stub.rotated != 1 || rotated.NotAfter.Before(time.Now().Add(29*24*time.Hour)) {
		t.Fatalf("rotate: %d %v", stub.rotated, rotated.NotAfter)
	}
	if rotated.Host.WG.EdgeEndpoint != "edge:51820" {
		t.Fatal("rotation lost host.json fields")
	}
	// The rotated cert authenticates against the api.
	if _, err := Rotate(context.Background(), cfg, rotated); err != nil {
		t.Fatalf("second rotate with the new certificate: %v", err)
	}
	// Reused token exits with the token error.
	_ = os.WriteFile(tokenPath, []byte("tok-1"), 0o600)
	_, err = Register(context.Background(), cfg)
	if err == nil || err.Error() != "register: join token already used: join token already used" {
		t.Fatalf("reused token: %v", err)
	}
	_ = os.Remove(tokenPath)
	if _, err := Register(context.Background(), cfg); err != ErrNoToken {
		t.Fatalf("no token: %v", err)
	}
	_ = x509.NewCertPool
}
