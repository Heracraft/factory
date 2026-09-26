package testca

import (
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/heracraft/repose/internal/ca/sshca"
)

func TestUserCertCarriesPrincipalsAndExtensions(t *testing.T) {
	ca, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, pub, err := sshca.GenerateHostKey("user")
	if err != nil {
		t.Fatal(err)
	}
	cert, err := ca.SignUserCert(pub, "u1:handle", []string{"p1", "p2"}, sshca.UserCertTTL)
	if err != nil {
		t.Fatal(err)
	}
	line := sshca.Marshal(cert)
	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	c := parsed.(*ssh.Certificate)
	if len(c.ValidPrincipals) != 2 || c.ValidPrincipals[0] != "p1" {
		t.Fatalf("principals %v", c.ValidPrincipals)
	}
	for ext := range sshca.UserExtensions {
		if _, ok := c.Extensions[ext]; !ok {
			t.Fatalf("missing %s", ext)
		}
	}
	checker := ssh.CertChecker{IsUserAuthority: func(k ssh.PublicKey) bool { return string(k.Marshal()) == string(ca.User.Signer.PublicKey().Marshal()) }}
	if err := checker.CheckCert("p1", c); err != nil {
		t.Fatal(err)
	}
	if err := checker.CheckCert("p3", c); err == nil {
		t.Fatal("cert accepted for a principal it does not carry")
	}
	if time.Until(time.Unix(int64(c.ValidBefore), 0)) > sshca.UserCertTTL+time.Hour {
		t.Fatal("validity too long")
	}
}

func TestHostKeyAndCert(t *testing.T) {
	ca, err := New()
	if err != nil {
		t.Fatal(err)
	}
	priv, certLine, err := ca.NewHostKey([]string{"10.64.4.2", "todo.heracraft"})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.ParsePrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(certLine))
	if err != nil {
		t.Fatal(err)
	}
	c := parsed.(*ssh.Certificate)
	if string(c.Key.Marshal()) != string(signer.PublicKey().Marshal()) {
		t.Fatal("certificate is not for the generated key")
	}
	if c.CertType != ssh.HostCert || c.ValidPrincipals[1] != "todo.heracraft" {
		t.Fatalf("host cert %+v", c)
	}
}
