// Package sshca holds the SSH certificate primitives shared by the api's
// CA (internal/api/ca) and the in-memory test CA (internal/ca/testca):
// ed25519 CA keys, user certificates with project principals and host
// certificates, per docs/interfaces/ssh-gateway.md.
package sshca

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// UserCertTTL is the lifetime of a user certificate (DECISIONS R3-9).
const UserCertTTL = 12 * time.Hour

// GatewayCertTTL is the lifetime of a gateway-issued certificate (I-1).
const GatewayCertTTL = 5 * time.Minute

// UserExtensions are the three extensions every user certificate carries.
var UserExtensions = map[string]string{
	"permit-pty":              "",
	"permit-port-forwarding":  "",
	"permit-agent-forwarding": "",
}

// Key is one CA key pair.
type Key struct {
	Signer ssh.Signer
}

// Generate makes a new ed25519 CA and returns it with its PEM private key.
func Generate(comment string) (*Key, []byte, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	blk, err := ssh.MarshalPrivateKey(priv, comment)
	if err != nil {
		return nil, nil, err
	}
	pemBytes := pem.EncodeToMemory(blk)
	k, err := Load(pemBytes)
	if err != nil {
		return nil, nil, err
	}
	return k, pemBytes, nil
}

// Load parses a PEM private key.
func Load(pemBytes []byte) (*Key, error) {
	s, err := ssh.ParsePrivateKey(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("ssh ca key: %w", err)
	}
	return &Key{Signer: s}, nil
}

// PublicLine is the CA public key as one authorized_keys line without a
// trailing newline.
func (k *Key) PublicLine() string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(k.Signer.PublicKey())))
}

// UserCert describes a user certificate to sign.
type UserCert struct {
	PublicKey   ssh.PublicKey
	KeyID       string
	Principals  []string
	Serial      uint64
	ValidAfter  time.Time
	ValidBefore time.Time
}

// SignUser signs a user certificate with the three standard extensions and
// no source-address restriction.
func (k *Key) SignUser(c UserCert) (*ssh.Certificate, error) {
	ext := map[string]string{}
	for name := range UserExtensions {
		ext[name] = ""
	}
	cert := &ssh.Certificate{
		Key:             c.PublicKey,
		Serial:          c.Serial,
		CertType:        ssh.UserCert,
		KeyId:           c.KeyID,
		ValidPrincipals: c.Principals,
		ValidAfter:      uint64(c.ValidAfter.Unix()),
		ValidBefore:     uint64(c.ValidBefore.Unix()),
		Permissions:     ssh.Permissions{Extensions: ext},
	}
	if err := cert.SignCert(rand.Reader, k.Signer); err != nil {
		return nil, err
	}
	return cert, nil
}

// HostCert describes a host certificate to sign.
type HostCert struct {
	PublicKey   ssh.PublicKey
	KeyID       string
	Principals  []string
	Serial      uint64
	ValidAfter  time.Time
	ValidBefore time.Time
}

// SignHost signs a host certificate.
func (k *Key) SignHost(c HostCert) (*ssh.Certificate, error) {
	cert := &ssh.Certificate{
		Key:             c.PublicKey,
		Serial:          c.Serial,
		CertType:        ssh.HostCert,
		KeyId:           c.KeyID,
		ValidPrincipals: c.Principals,
		ValidAfter:      uint64(c.ValidAfter.Unix()),
		ValidBefore:     uint64(c.ValidBefore.Unix()),
	}
	if err := cert.SignCert(rand.Reader, k.Signer); err != nil {
		return nil, err
	}
	return cert, nil
}

// Marshal renders a certificate as one authorized_keys line without a
// trailing newline (the OpenSSH `-cert.pub` format).
func Marshal(cert *ssh.Certificate) string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(cert)))
}

// GenerateHostKey makes an ed25519 sshd host key: the PEM private key and
// its public key.
func GenerateHostKey(comment string) ([]byte, ssh.PublicKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	blk, err := ssh.MarshalPrivateKey(priv, comment)
	if err != nil {
		return nil, nil, err
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(blk), sshPub, nil
}

// ParsePublicKey parses one authorized_keys line (a plain key or a
// certificate's underlying key).
func ParsePublicKey(line string) (ssh.PublicKey, error) {
	pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(line)))
	if err != nil {
		return nil, fmt.Errorf("public key: %w", err)
	}
	if c, ok := pk.(*ssh.Certificate); ok {
		return c.Key, nil
	}
	return pk, nil
}

// Fingerprint is the SHA256 fingerprint of a key.
func Fingerprint(pk ssh.PublicKey) string { return ssh.FingerprintSHA256(pk) }
