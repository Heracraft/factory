package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func TestIssueAndVerify(t *testing.T) {
	ca, err := Generate("repose host ca")
	if err != nil {
		t.Fatal(err)
	}
	keyPEM, err := ca.KeyPEM()
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(ca.CertPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	certPEM, _, serial, err := reloaded.IssueClient("host-01", HostCertValidity)
	if err != nil || serial == "" {
		t.Fatal(err)
	}
	blk, _ := pem.Decode(certPEM)
	leaf, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if leaf.Subject.CommonName != "host-01" {
		t.Fatalf("cn %q", leaf.Subject.CommonName)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: ca.Pool(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		t.Fatal(err)
	}
	other, _ := Generate("other")
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: other.Pool(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err == nil {
		t.Fatal("verified against the wrong CA")
	}
}

// A CSR made elsewhere (the edge's gateway key) gets a certificate the CA
// verifies, for the CSR's key, with the requested names; the request's own
// subject is ignored and a tampered request is refused (I-92).
func TestSignCSR(t *testing.T) {
	ca, err := Generate("repose test ca")
	if err != nil {
		t.Fatal(err)
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "ignored"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})

	certPEM, serial, err := ca.SignCSR(csrPEM, false, time.Hour, "gateway")
	if err != nil || serial == "" {
		t.Fatalf("client csr: %v", err)
	}
	b, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if cert.Subject.CommonName != "gateway" || !key.PublicKey.Equal(cert.PublicKey) || cert.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth {
		t.Fatalf("client cert: cn %q eku %v", cert.Subject.CommonName, cert.ExtKeyUsage)
	}
	if _, err := cert.Verify(x509.VerifyOptions{Roots: ca.Pool(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		t.Fatalf("verify: %v", err)
	}

	srvPEM, _, err := ca.SignCSR(csrPEM, true, time.Hour, "10.255.0.1", "edge.internal")
	if err != nil {
		t.Fatalf("server csr: %v", err)
	}
	b, _ = pem.Decode(srvPEM)
	srv, _ := x509.ParseCertificate(b.Bytes)
	if len(srv.IPAddresses) != 1 || srv.IPAddresses[0].String() != "10.255.0.1" || len(srv.DNSNames) != 1 || srv.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Fatalf("server cert SANs: %v %v", srv.IPAddresses, srv.DNSNames)
	}

	if _, _, err := ca.SignCSR(csrPEM, false, time.Hour); err == nil {
		t.Fatal("no name should refuse")
	}
	bad := []byte(strings.Replace(string(csrPEM), "A", "B", 1))
	if _, _, err := ca.SignCSR(bad, false, time.Hour, "gateway"); err == nil {
		t.Fatal("tampered request should refuse")
	}
	if _, _, err := ca.SignCSR([]byte("-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n"), false, time.Hour, "x"); err == nil {
		t.Fatal("a certificate is not a request")
	}
}
