package app_test

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/api/app"
	"github.com/heracraft/repose/internal/db/testdb"
)

func TestMain(m *testing.M) { os.Exit(testdb.Run(m)) }

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

// The whole process in dev mode: /healthz, /internal refusing a client
// without the gateway certificate and accepting one with it, graceful
// shutdown.
func TestProcessDevModeAndInternalMTLS(t *testing.T) {
	url := testdb.URL(t)
	cfg := app.Config{Mode: "all", Listen: freePort(t), GRPCListen: freePort(t), InternalListen: freePort(t), MetricsListen: freePort(t),
		DatabaseURL: url, Dev: true, APIResource: "https://api.test", GatewayHost: "ssh.test", GatewayPort: 22, ReplicaID: "test", GRPCServerNames: []string{"127.0.0.1"}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a, err := app.New(ctx, cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if res, err := http.Get("http://" + cfg.Listen + "/healthz"); err == nil {
			_ = res.Body.Close()
			if res.StatusCode == 200 {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	res, err := http.Get("http://" + cfg.Listen + "/healthz")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("healthz: %v %v", res, err)
	}
	_ = res.Body.Close()
	// /internal without a client certificate: the TLS handshake fails.
	noCert := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, Timeout: 5 * time.Second}
	if _, err := noCert.Get("https://" + cfg.InternalListen + "/v1/internal/ca"); err == nil {
		t.Fatal("internal route served without a client certificate")
	} else {
		t.Logf("without client cert: %v", err)
	}
	// With a certificate from the host CA (what `repose-admin ca sign-client --name gateway` issues).
	clientCert, err := a.HostCA().ClientTLS("gateway")
	if err != nil {
		t.Fatal(err)
	}
	withCert := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: a.HostCA().Pool(), ServerName: "127.0.0.1", Certificates: []tls.Certificate{clientCert}}}, Timeout: 5 * time.Second}
	res, err = withCert.Get("https://" + cfg.InternalListen + "/v1/internal/ca")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "user_ca_pub") {
		t.Fatalf("with client cert: %d %s", res.StatusCode, body)
	}
	t.Logf("with client cert: %d %s", res.StatusCode, body)
	res, err = http.Get("http://" + cfg.MetricsListen + "/metrics")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("metrics: %v", err)
	}
	mb, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if !strings.Contains(string(mb), "repose_api_grpc_streams") || !strings.Contains(string(mb), "repose_api_requests_total") {
		t.Fatal("metrics missing repose_api_ families")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("shutdown did not complete")
	}
}
