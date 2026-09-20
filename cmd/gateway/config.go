package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/heracraft/repose/internal/gateway"
)

// env reads a variable or a default.
func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// mustEnv reads a required variable.
func mustEnv(key string) (string, error) {
	if v := os.Getenv(key); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("%s is required", key)
}

// apiClient builds the internal-routes client from API_URL and the mTLS
// client certificate (GATEWAY_CLIENT_CERT/KEY), trusting API_CA for the
// api's /internal server certificate (I-42) or the system roots when it is
// unset (a public certificate).
func apiClient() (*gateway.Client, error) {
	base, err := mustEnv("API_URL")
	if err != nil {
		return nil, err
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if cert, key := os.Getenv("GATEWAY_CLIENT_CERT"), os.Getenv("GATEWAY_CLIENT_KEY"); cert != "" && key != "" {
		pair, err := tls.LoadX509KeyPair(cert, key)
		if err != nil {
			return nil, fmt.Errorf("gateway client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{pair}
	}
	if caPath := os.Getenv("API_CA"); caPath != "" {
		pem, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("API_CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("API_CA: no certificate in %s", caPath)
		}
		tlsCfg.RootCAs = pool
	}
	return gateway.NewClient(base, tlsCfg, 10*time.Second)
}

// hostSigner loads the gateway's host key and its Host-CA certificate
// (HOST_KEY, HOST_CERT) into a cert signer clients verify without a
// known_hosts prompt (ssh-gateway.md). With no certificate it presents the
// bare key.
func hostSigner() (ssh.Signer, error) {
	keyPath, err := mustEnv("HOST_KEY")
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("HOST_KEY: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(keyPEM)
	if err != nil {
		return nil, fmt.Errorf("HOST_KEY: %w", err)
	}
	certPath := os.Getenv("HOST_CERT")
	if certPath == "" {
		return signer, nil
	}
	certBytes, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("HOST_CERT: %w", err)
	}
	pk, _, _, _, err := ssh.ParseAuthorizedKey(certBytes)
	if err != nil {
		return nil, fmt.Errorf("HOST_CERT: %w", err)
	}
	cert, ok := pk.(*ssh.Certificate)
	if !ok {
		return nil, fmt.Errorf("HOST_CERT: not a certificate")
	}
	return ssh.NewCertSigner(cert, signer)
}

// gatewaySigner loads the gateway's own SSH key pair used to dial guests
// with a per-project 5-minute certificate (GATEWAY_SSH_KEY). When the path
// is unset or missing, an ephemeral key is generated: the api certifies
// whatever public key the gateway sends per request, so a fresh key only
// costs the 5-minute cert cache on a restart.
func gatewaySigner() (ssh.Signer, error) {
	if path := os.Getenv("GATEWAY_SSH_KEY"); path != "" {
		if b, err := os.ReadFile(path); err == nil {
			return ssh.ParsePrivateKey(b)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("GATEWAY_SSH_KEY: %w", err)
		}
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(priv)
}
