package hostdev

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHCA signs guest host certificates and the operator's user
// certificates, standing in for the api's CA until workstream 05 exists.
type SSHCA struct {
	Signer ssh.Signer
	Pub    string // authorized_keys line
}

// NewSSHCA generates an ed25519 CA and writes it under dir.
func NewSSHCA(dir string) (*SSHCA, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	blk, err := ssh.MarshalPrivateKey(priv, "repose hostdev ssh ca")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, SSHCAKey), pem.EncodeToMemory(blk), 0o600); err != nil {
		return nil, err
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, err
	}
	line := string(ssh.MarshalAuthorizedKey(sshPub))
	if err := os.WriteFile(filepath.Join(dir, SSHCAPub), []byte(line), 0o644); err != nil {
		return nil, err
	}
	return LoadSSHCA(dir)
}

// LoadSSHCA reads the CA from dir.
func LoadSSHCA(dir string) (*SSHCA, error) {
	kb, err := os.ReadFile(filepath.Join(dir, SSHCAKey))
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(kb)
	if err != nil {
		return nil, fmt.Errorf("ssh ca key: %w", err)
	}
	pb, err := os.ReadFile(filepath.Join(dir, SSHCAPub))
	if err != nil {
		return nil, err
	}
	return &SSHCA{Signer: signer, Pub: string(pb)}, nil
}

func (c *SSHCA) sign(cert *ssh.Certificate) ([]byte, error) {
	if err := cert.SignCert(rand.Reader, c.Signer); err != nil {
		return nil, err
	}
	return ssh.MarshalAuthorizedKey(cert), nil
}

// GuestHostKey generates a guest sshd key and its Host-CA certificate with
// principals guest_ip and <slug>.<handle>, as CreateGuest carries them.
func (c *SSHCA) GuestHostKey(guestIP, slug, handle string) (key, cert []byte, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	blk, err := ssh.MarshalPrivateKey(priv, "repose guest host key")
	if err != nil {
		return nil, nil, err
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, nil, err
	}
	principals := []string{}
	if guestIP != "" {
		principals = append(principals, guestIP)
	}
	if slug != "" && handle != "" {
		principals = append(principals, slug+"."+handle)
	}
	hc := &ssh.Certificate{
		Key: sshPub, CertType: ssh.HostCert, KeyId: "guest-" + slug, ValidPrincipals: principals,
		ValidAfter: uint64(time.Now().Add(-time.Minute).Unix()), ValidBefore: uint64(time.Now().Add(5 * 365 * 24 * time.Hour).Unix()),
	}
	certLine, err := c.sign(hc)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(blk), certLine, nil
}

// UserCert signs the operator's public key for one project: principal
// project_id, 12 hours, the three extensions from DESIGN.md §8.
func (c *SSHCA) UserCert(pubLine []byte, projectID string) ([]byte, error) {
	pub, _, _, _, err := ssh.ParseAuthorizedKey(pubLine)
	if err != nil {
		return nil, fmt.Errorf("public key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, bigMax)
	if err != nil {
		return nil, err
	}
	uc := &ssh.Certificate{
		Key: pub, CertType: ssh.UserCert, KeyId: "hostdev-" + projectID, Serial: serial.Uint64(), ValidPrincipals: []string{projectID},
		ValidAfter: uint64(time.Now().Add(-time.Minute).Unix()), ValidBefore: uint64(time.Now().Add(12 * time.Hour).Unix()),
		Permissions: ssh.Permissions{Extensions: map[string]string{"permit-agent-forwarding": "", "permit-port-forwarding": "", "permit-pty": ""}},
	}
	return c.sign(uc)
}
