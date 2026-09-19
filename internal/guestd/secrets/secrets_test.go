package secrets

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
	"github.com/heracraft/repose/internal/guestd/sysdep"
)

func quietLog() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func newHandler(t *testing.T) (*Handler, sysdep.Paths, *sysdep.FakeRunner) {
	t.Helper()
	p := sysdep.Paths{Root: t.TempDir()}
	run := sysdep.NewFakeRunner()
	h := New(p, run, quietLog())
	// The test process is not root, so it cannot chown to dev; ownership is
	// asserted in the NixOS VM test instead.
	h.uid, h.gid = -1, -1
	return h, p, run
}

func secret(name, value string) *guestdv1.Secret {
	return &guestdv1.Secret{Name: name, Value: []byte(value)}
}

func TestWriteSecretsFileModes(t *testing.T) {
	h, p, _ := newHandler(t)
	err := h.Write(context.Background(), []*guestdv1.Secret{
		secret("OPENAI_API_KEY", "sk-abc"),
		secret("GEMINI_API_KEY", "gm-def"),
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	for _, name := range []string{"OPENAI_API_KEY", "GEMINI_API_KEY"} {
		fi, err := os.Stat(filepath.Join(p.SecretsDir(), name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if fi.Mode().Perm() != 0o400 {
			t.Errorf("%s mode = %o, want 400", name, fi.Mode().Perm())
		}
	}
	fi, err := os.Stat(p.SecretsEnv())
	if err != nil {
		t.Fatalf("secrets.env: %v", err)
	}
	if fi.Mode().Perm() != 0o400 {
		t.Errorf("secrets.env mode = %o, want 400", fi.Mode().Perm())
	}
}

func TestSecretsEnvQuoting(t *testing.T) {
	h, p, _ := newHandler(t)
	// A value with a single quote and newlines is the case that breaks a naive
	// writer and leaves every login shell broken.
	value := "it's a\nmulti 'line' value\n"
	if err := h.Write(context.Background(), []*guestdv1.Secret{secret("TRICKY", value)}); err != nil {
		t.Fatalf("write: %v", err)
	}
	b, err := os.ReadFile(p.SecretsEnv())
	if err != nil {
		t.Fatal(err)
	}
	want := "export TRICKY='it'\\''s a\nmulti '\\''line'\\'' value\n'\n"
	if !strings.Contains(string(b), want) {
		t.Fatalf("secrets.env =\n%q\nwant it to contain\n%q", b, want)
	}
	// And the quoting must actually survive a shell.
	if got := unquote(t, string(b)); got != value {
		t.Fatalf("a shell sourcing the file reads %q, want %q", got, value)
	}
}

// unquote runs the env file through sh and prints the variable back, which is
// the only check that matters for shell quoting.
func unquote(t *testing.T, env string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.env")
	if err := os.WriteFile(path, []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := sysdep.ExecRunner{}.Run(context.Background(), sysdep.RunSpec{
		Argv: []string{"sh", "-c", ". " + path + `; printf %s "$TRICKY"`},
	})
	if err != nil {
		t.Fatalf("sh: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("sh exited %d: %s", res.ExitCode, res.Stderr)
	}
	return string(res.Stdout)
}

func TestReservedNamesGoToRunDirAndReloadSSHD(t *testing.T) {
	h, p, run := newHandler(t)
	err := h.Write(context.Background(), []*guestdv1.Secret{
		secret(ReservedHostKey, "PRIVATE KEY"),
		secret(ReservedHostCert, "ssh-ed25519-cert..."),
		secret(ReservedUserCA, "ssh-ed25519 AAAA..."),
		secret("APP_TOKEN", "t"),
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	cases := []struct {
		name string
		mode os.FileMode
	}{
		{ReservedHostKey, 0o600},
		{ReservedHostCert, 0o644},
		{ReservedUserCA, 0o644},
	}
	for _, c := range cases {
		fi, err := os.Stat(filepath.Join(p.RunDir(), c.name))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if fi.Mode().Perm() != c.mode {
			t.Errorf("%s mode = %o, want %o", c.name, fi.Mode().Perm(), c.mode)
		}
		if _, err := os.Stat(filepath.Join(p.SecretsDir(), c.name)); !os.IsNotExist(err) {
			t.Errorf("%s was also written into the secrets directory", c.name)
		}
	}

	b, err := os.ReadFile(p.SecretsEnv())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "ssh_host") || strings.Contains(string(b), "user_ca") {
		t.Fatalf("sshd material leaked into secrets.env:\n%s", b)
	}
	if _, ok := run.Ran("reload-or-restart sshd.service"); !ok {
		t.Fatalf("sshd was not reloaded; calls: %v", run.Calls())
	}
}

func TestWriteIsTheWholeSetSoRemovalReachesTheGuest(t *testing.T) {
	h, p, _ := newHandler(t)
	ctx := context.Background()
	if err := h.Write(ctx, []*guestdv1.Secret{secret("A", "1"), secret("B", "2")}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := h.Write(ctx, []*guestdv1.Secret{secret("A", "1")}); err != nil {
		t.Fatalf("second write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.SecretsDir(), "B")); !os.IsNotExist(err) {
		t.Fatal("a withdrawn secret was left on the tmpfs")
	}
	b, _ := os.ReadFile(p.SecretsEnv())
	if strings.Contains(string(b), "export B=") {
		t.Fatalf("a withdrawn secret is still exported:\n%s", b)
	}
}

func TestWriteIsAtomicPerRequest(t *testing.T) {
	h, p, _ := newHandler(t)
	ctx := context.Background()
	if err := h.Write(ctx, []*guestdv1.Secret{secret("GOOD", "1")}); err != nil {
		t.Fatalf("write: %v", err)
	}

	oversized := strings.Repeat("x", MaxValueBytes+1)
	err := h.Write(ctx, []*guestdv1.Secret{secret("ALSO_GOOD", "2"), secret("TOO_BIG", oversized)})
	if err == nil {
		t.Fatal("an oversized value was accepted")
	}
	if sysdep.CodeOf(err) != sysdep.CodeInvalidArgument {
		t.Fatalf("code = %s, want invalid_argument", sysdep.CodeOf(err))
	}
	if _, err := os.Stat(filepath.Join(p.SecretsDir(), "ALSO_GOOD")); !os.IsNotExist(err) {
		t.Fatal("part of a rejected batch was written")
	}
	if _, err := os.Stat(filepath.Join(p.SecretsDir(), "GOOD")); err != nil {
		t.Fatal("the rejected batch disturbed an existing secret")
	}
}

func TestInvalidNames(t *testing.T) {
	h, _, _ := newHandler(t)
	bad := []string{"", "lower-case-dash", "9LEADING", "HAS SPACE", "../escape", "DUP"}
	for _, name := range bad {
		list := []*guestdv1.Secret{secret(name, "v")}
		if name == "DUP" {
			list = append(list, secret("DUP", "v2"))
		}
		if err := h.Write(context.Background(), list); err == nil {
			t.Errorf("%q was accepted", name)
		} else if sysdep.CodeOf(err) != sysdep.CodeInvalidArgument {
			t.Errorf("%q: code = %s, want invalid_argument", name, sysdep.CodeOf(err))
		}
	}
}

func TestNulByteInValueRejected(t *testing.T) {
	h, _, _ := newHandler(t)
	err := h.Write(context.Background(), []*guestdv1.Secret{{Name: "NULLY", Value: []byte("a\x00b")}})
	if sysdep.CodeOf(err) != sysdep.CodeInvalidArgument {
		t.Fatalf("code = %s, want invalid_argument", sysdep.CodeOf(err))
	}
}

func TestReservedNamesAreNotRemovedByALaterWrite(t *testing.T) {
	h, p, _ := newHandler(t)
	ctx := context.Background()
	if err := h.Write(ctx, []*guestdv1.Secret{secret(ReservedUserCA, "ca")}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := h.Write(ctx, []*guestdv1.Secret{secret("A", "1")}); err != nil {
		t.Fatalf("second write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.RunDir(), ReservedUserCA)); err != nil {
		t.Fatal("the CA public key was removed by an unrelated secrets update")
	}
}

func TestIsReserved(t *testing.T) {
	for _, n := range []string{ReservedHostKey, ReservedHostCert, ReservedUserCA} {
		if !IsReserved(n) {
			t.Errorf("%s is not reported reserved", n)
		}
	}
	if IsReserved("ANTHROPIC_API_KEY") {
		t.Error("a normal secret name is reported reserved")
	}
}
