package ops

import (
	"golang.org/x/crypto/ssh"
)

// parseSSHKey returns the public key of a PEM private key.
func parseSSHKey(pemBytes []byte) (ssh.PublicKey, error) {
	s, err := ssh.ParsePrivateKey(pemBytes)
	if err != nil {
		return nil, err
	}
	return s.PublicKey(), nil
}
