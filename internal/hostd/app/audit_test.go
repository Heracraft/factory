package app

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// The PAM hook reads identifiers out of SSH_AUTH_INFO_0 and nothing else:
// a certificate's key id and serial, a key's fingerprint, never a body
// (docs/ops/OBSERVABILITY.md "Never in a log field"; I-140).
func TestParseAuthInfo(t *testing.T) {
	caPub, caPriv, _ := ed25519.GenerateKey(rand.Reader)
	caSigner, _ := ssh.NewSignerFromKey(caPriv)
	_ = caPub
	opPub, _, _ := ed25519.GenerateKey(rand.Reader)
	opKey, _ := ssh.NewPublicKey(opPub)
	cert := &ssh.Certificate{Key: opKey, Serial: 4242, CertType: ssh.UserCert, KeyId: "operator:alice", ValidPrincipals: []string{"root"}, ValidBefore: ssh.CertTimeInfinity}
	if err := cert.SignCert(rand.Reader, caSigner); err != nil {
		t.Fatal(err)
	}
	certB64 := base64.StdEncoding.EncodeToString(cert.Marshal())
	info := "publickey " + cert.Type() + " " + certB64 + "\n"

	keyID, serial, fp := ParseAuthInfo(info)
	if keyID != "operator:alice" || serial != 4242 {
		t.Fatalf("certificate: key_id %q serial %d", keyID, serial)
	}
	if fp != ssh.FingerprintSHA256(opKey) || !strings.HasPrefix(fp, "SHA256:") {
		t.Fatalf("fingerprint %q", fp)
	}
	for _, out := range []string{keyID, fp} {
		if strings.Contains(certB64, out) || strings.Contains(out, certB64[:20]) {
			t.Fatalf("a body leaked into %q", out)
		}
	}

	plainB64 := base64.StdEncoding.EncodeToString(opKey.Marshal())
	keyID, serial, fp = ParseAuthInfo("publickey ssh-ed25519 " + plainB64)
	if keyID != "" || serial != 0 || fp != ssh.FingerprintSHA256(opKey) {
		t.Fatalf("plain key: %q %d %q", keyID, serial, fp)
	}

	for _, bad := range []string{"", "password", "publickey ssh-ed25519 not-base64", "publickey ssh-ed25519"} {
		if k, s, f := ParseAuthInfo(bad); k != "" || s != 0 || f != "" {
			t.Fatalf("%q parsed to %q %d %q", bad, k, s, f)
		}
	}
}
