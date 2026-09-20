package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const certReuseMargin = 30 * time.Minute

// ensureIdentityKey makes sure ~/.ssh/id_ed25519(.pub) exists, generating
// one with ssh-keygen if not (07-cli.md §5.4: "never touch an existing
// key"). It returns the public key line.
func ensureIdentityKey() (string, error) {
	priv, err := userIdentityFile()
	if err != nil {
		return "", err
	}
	pub := priv + ".pub"
	if _, err := os.Stat(priv); err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(priv), 0o700); err != nil {
			return "", err
		}
		cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-C", "repose", "-f", priv)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("ssh-keygen: %w: %s", err, out)
		}
	}
	b, err := os.ReadFile(pub)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// parseCertFile reads an OpenSSH certificate written by a previous
// ensureCert. A missing or unparsable file is not an error: it just means
// there is nothing to reuse.
func parseCertFile(path string) *ssh.Certificate {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey(b)
	if err != nil {
		return nil
	}
	cert, ok := pub.(*ssh.Certificate)
	if !ok {
		return nil
	}
	return cert
}

// certUsableFor reports whether cert is valid for at least margin more and
// carries every id in want as a principal.
func certUsableFor(cert *ssh.Certificate, want []string, now time.Time, margin time.Duration) bool {
	if cert == nil {
		return false
	}
	if time.Unix(int64(cert.ValidBefore), 0).Sub(now) <= margin {
		return false
	}
	have := map[string]bool{}
	for _, p := range cert.ValidPrincipals {
		have[p] = true
	}
	for _, id := range want {
		if !have[id] {
			return false
		}
	}
	return true
}

// certParams is what ensureCert needs about the account to write the SSH
// config and known_hosts.
type certParams struct {
	Handle   string
	Projects []Project // every project the user has; the cert covers all of them
}

// ensureCert implements 07-cli.md §5.4: reuse a valid certificate, else
// issue a new one covering every project the user has, write it and the
// known_hosts and ssh config files, and register the Include line. It
// returns the certificate path.
func ensureCert(ctx context.Context, client *Client, params certParams, now func() time.Time) (string, error) {
	if now == nil {
		now = time.Now
	}
	sd, err := sshDir()
	if err != nil {
		return "", err
	}
	certPath := filepath.Join(sd, "id_ed25519-cert.pub")

	ids := make([]string, 0, len(params.Projects))
	for _, p := range params.Projects {
		ids = append(ids, p.ID)
	}
	sort.Strings(ids)

	pubLine, err := ensureIdentityKey()
	if err != nil {
		return "", err
	}

	existing := parseCertFile(certPath)
	if certUsableFor(existing, ids, now(), certReuseMargin) {
		return certPath, writeSSHFiles(sd, params, hostCAFromCert(existing))
	}

	resp, err := client.IssueCert(ctx, pubLine, ids)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Code == "rate_limited" {
			if existing != nil && time.Unix(int64(existing.ValidBefore), 0).After(now()) {
				_, _ = fmt.Fprintf(os.Stderr, "warning: certificate rate limited; reusing the one on disk (%s left)\n", time.Unix(int64(existing.ValidBefore), 0).Sub(now()))
				return certPath, writeSSHFiles(sd, params, hostCAFromCert(existing))
			}
		}
		return "", fmt.Errorf("issuing certificate: %w", err)
	}

	if err := writeFileAtomic(certPath, []byte(resp.Certificate+"\n"), 0o600); err != nil {
		return "", err
	}
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		_ = exec.Command("ssh-add", certPath).Run() // best effort per 5.4
	}
	if err := writeSSHFiles(sd, params, resp.Gateway.HostCAPub); err != nil {
		return "", err
	}
	return certPath, nil
}

// hostCAFromCert has no way to recover the host CA public key from a
// reused user certificate (it only signs user certs), so a reuse leaves
// known_hosts untouched by passing "" through writeSSHFiles.
func hostCAFromCert(*ssh.Certificate) string { return "" }

func writeSSHFiles(sshDirPath string, params certParams, hostCAPub string) error {
	if hostCAPub != "" {
		known := fmt.Sprintf("@cert-authority %s,10.64.* %s\n", gatewayHost, hostCAPub)
		if err := writeFileAtomic(filepath.Join(sshDirPath, "known_hosts"), []byte(known), 0o600); err != nil {
			return err
		}
	}
	cfg := renderSSHConfig(params.Projects, params.Handle)
	if err := writeFileAtomic(filepath.Join(sshDirPath, "config"), []byte(cfg), 0o600); err != nil {
		return err
	}
	usc, err := userSSHConfig()
	if err != nil {
		return err
	}
	return ensureIncludeLine(usc)
}

const gatewayHost = "ssh.repose.herakraft.co"

// renderSSHConfig builds ~/.ssh/repose/config: one Host block per project,
// in slug order for a stable diff (docs/interfaces/ssh-gateway.md "CLI
// side").
func renderSSHConfig(projects []Project, handle string) string {
	sorted := append([]Project(nil), projects...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Slug < sorted[j].Slug })

	var b strings.Builder
	b.WriteString("# Generated by repose; edits here are overwritten on the next `repose run`.\n")
	for _, p := range sorted {
		_, _ = fmt.Fprintf(&b, "Host %s.repose\n", p.Slug)
		_, _ = fmt.Fprintf(&b, "  HostName %s\n", gatewayHost)
		_, _ = fmt.Fprintf(&b, "  User %s.%s\n", p.Slug, handle)
		b.WriteString("  CertificateFile ~/.ssh/repose/id_ed25519-cert.pub\n")
		b.WriteString("  IdentityFile ~/.ssh/id_ed25519\n")
		b.WriteString("  UserKnownHostsFile ~/.ssh/repose/known_hosts\n")
		b.WriteString("  ForwardAgent yes\n")
		// Only the certificate identity above: with an agent loaded, ssh
		// otherwise offers the agent's plain keys first and the gateway
		// answers "certificate required" (M2 gate, DECISIONS I-108).
		b.WriteString("  IdentitiesOnly yes\n")
		b.WriteString("  ServerAliveInterval 30\n")
	}
	return b.String()
}
