package pki

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
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
