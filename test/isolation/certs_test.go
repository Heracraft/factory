package isolation

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func sshBatch(t *testing.T, key, cert, target string, timeout time.Duration) result {
	t.Helper()
	host, port, _ := strings.Cut(env("GATEWAY"), ":")
	if port == "" {
		port = "22"
	}
	args := []string{"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=10", "-i", key, "-o", "CertificateFile=" + cert, "-p", port, target + "@" + host, "true"}
	cmd := exec.Command("ssh", args...)
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	var err error
	select {
	case err = <-done:
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		err = <-done
	}
	r := result{out: out.String(), err: errb.String()}
	if ee, ok := err.(*exec.ExitError); ok {
		r.code = ee.ExitCode()
	} else if err != nil {
		r.code = -1
	}
	return r
}

// Row: A's certificate cannot open B. Gateway half: the route lookup plus
// principal match rejects with the documented banner.
func TestCertificateForACannotOpenBAtGateway(t *testing.T) {
	need(t, "GATEWAY", "KEY_A", "CERT_A", "LOGIN_A", "LOGIN_B")
	mustSucceed(t, sshBatch(t, env("KEY_A"), env("CERT_A"), env("LOGIN_A"), 30*time.Second), "A's certificate opens A")
	r := sshBatch(t, env("KEY_A"), env("CERT_A"), env("LOGIN_B"), 30*time.Second)
	mustFail(t, r, "A's certificate at the gateway for B")
	if !strings.Contains(r.err, "certificate not valid for this project") {
		t.Fatalf("gateway refused without the documented banner:\n%s", r)
	}
}

// Row, guest half: B's sshd itself rejects A's certificate (principal is
// the project id, AuthorizedPrincipalsFile). Run from the host with the
// runbook's temporary input rule in place, hence the opt-in.
func TestCertificateForACannotOpenBDirect(t *testing.T) {
	need(t, "HOST_EXEC", "B_IP", "KEY_A", "CERT_A", "DIRECT_SSH")
	mustSucceed(t, onHost(t, "test -r "+env("KEY_A")+" && test -r "+env("CERT_A")), "A's key and certificate readable on the host")
	r := onHost(t, "ssh -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=10 -i "+env("KEY_A")+" -o CertificateFile="+env("CERT_A")+" dev@"+env("B_IP")+" true")
	mustFail(t, r, "A's certificate at B's sshd")
}

// Row: an expired or revoked certificate is rejected within 30 s.
func TestRevokedCertificateRejected(t *testing.T) {
	need(t, "GATEWAY", "KEY_A", "CERT_A", "LOGIN_A", "API_URL", "TOKEN_A")
	serialOut, err := exec.Command("ssh-keygen", "-L", "-f", env("CERT_A")).Output()
	if err != nil {
		t.Fatalf("ssh-keygen -L: %v", err)
	}
	serial := ""
	for _, line := range strings.Split(string(serialOut), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == "Serial:" {
			serial = f[1]
		}
	}
	if serial == "" {
		t.Fatalf("no serial in certificate:\n%s", serialOut)
	}
	mustSucceed(t, sshBatch(t, env("KEY_A"), env("CERT_A"), env("LOGIN_A"), 30*time.Second), "A's certificate before revocation")
	body := `{"serial":` + serial + `}`
	curl := exec.Command("curl", "-sS", "-f", "-X", "POST", "-H", "Authorization: Bearer "+env("TOKEN_A"), "-H", "Content-Type: application/json", "-d", body, env("API_URL")+"/certs/revoke")
	if out, err := curl.CombinedOutput(); err != nil {
		t.Fatalf("revoke: %v\n%s", err, out)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		r := sshBatch(t, env("KEY_A"), env("CERT_A"), env("LOGIN_A"), 20*time.Second)
		if r.code != 0 {
			t.Logf("revoked certificate rejected %s after revocation", time.Until(deadline).Round(time.Second))
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("revoked certificate still accepted 30 s after revocation")
		}
		time.Sleep(2 * time.Second)
	}
}
