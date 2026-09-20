// Package testca generates both SSH CAs in memory and signs certificates
// for tests (docs/interfaces/ssh-gateway.md "Test CA"). The gateway and
// guest sshd tests use it; the api's own CA is internal/api/ca.
package testca

import (
	"fmt"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/heracraft/repose/internal/ca/sshca"
)

// CA holds the User CA and Host CA.
type CA struct {
	User   *sshca.Key
	Host   *sshca.Key
	serial atomic.Uint64
}

// New generates both CAs.
func New() (*CA, error) {
	u, _, err := sshca.Generate("repose test user ca")
	if err != nil {
		return nil, err
	}
	h, _, err := sshca.Generate("repose test host ca")
	if err != nil {
		return nil, err
	}
	return &CA{User: u, Host: h}, nil
}

// SignUserCert signs a user certificate for the key with the project ids
// as principals, valid for ttl from now.
func (c *CA) SignUserCert(pub ssh.PublicKey, keyID string, projectIDs []string, ttl time.Duration) (*ssh.Certificate, error) {
	now := time.Now()
	return c.User.SignUser(sshca.UserCert{PublicKey: pub, KeyID: keyID, Principals: projectIDs, Serial: c.serial.Add(1), ValidAfter: now.Add(-time.Minute), ValidBefore: now.Add(ttl)})
}

// SignHostCert signs a host certificate with the given principals.
func (c *CA) SignHostCert(pub ssh.PublicKey, keyID string, principals []string, ttl time.Duration) (*ssh.Certificate, error) {
	now := time.Now()
	return c.Host.SignHost(sshca.HostCert{PublicKey: pub, KeyID: keyID, Principals: principals, Serial: c.serial.Add(1), ValidAfter: now.Add(-time.Minute), ValidBefore: now.Add(ttl)})
}

// NewHostKey generates an ed25519 host key and signs its certificate, as
// the api does for a guest before CreateGuest (DECISIONS I-3).
func (c *CA) NewHostKey(principals []string) (privPEM []byte, certLine string, err error) {
	priv, pub, err := sshca.GenerateHostKey("repose test host key")
	if err != nil {
		return nil, "", err
	}
	cert, err := c.SignHostCert(pub, fmt.Sprintf("host:%s", principals[0]), principals, 10*365*24*time.Hour)
	if err != nil {
		return nil, "", err
	}
	return priv, sshca.Marshal(cert), nil
}
