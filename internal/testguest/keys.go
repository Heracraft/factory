package testguest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"

	"golang.org/x/crypto/ssh"
)

func generateEd25519() (privPEM []byte, pub ssh.PublicKey, err error) {
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	block, err := ssh.MarshalPrivateKey(sk, "testguest")
	if err != nil {
		return nil, nil, err
	}
	sshPub, err := ssh.NewPublicKey(pk)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(block), sshPub, nil
}

func generateHostKey() (ssh.Signer, error) {
	_, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(sk)
}
