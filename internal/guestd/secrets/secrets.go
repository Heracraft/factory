// Package secrets implements the WriteSecrets request of
// docs/interfaces/vsock-guestd.md: named secret values onto the tmpfs at
// /run/repose/secrets, the shell env file beside them, and the three reserved
// sshd names of DECISIONS I-10.
//
// Nothing in this package logs a secret name together with a value, and
// nothing logs a value at all. Counts and error codes are what comes out.
package secrets

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
	"github.com/heracraft/repose/internal/guestd/sysdep"
)

// MaxValueBytes is the per-secret cap (docs/workstreams/04-guestd.md).
const MaxValueBytes = 64 << 10

// MaxNameBytes bounds a secret name so a name cannot be used as a payload.
const MaxNameBytes = 128

// Reserved names carry the guest's sshd material rather than a tenant secret.
// DECISIONS I-10: hostd delivers them through WriteSecrets and they land in
// /run/repose, not in the secrets directory, and never in secrets.env.
const (
	ReservedHostKey  = "ssh_host_ed25519_key"
	ReservedHostCert = "ssh_host_ed25519_key-cert.pub"
	ReservedUserCA   = "user_ca.pub"
)

type reservedSpec struct {
	file string
	mode os.FileMode
}

var reserved = map[string]reservedSpec{
	ReservedHostKey:  {file: ReservedHostKey, mode: 0o600},
	ReservedHostCert: {file: ReservedHostCert, mode: 0o644},
	ReservedUserCA:   {file: ReservedUserCA, mode: 0o644},
}

// nameRe is the shape of a named secret: it becomes a shell variable, so it
// must be a shell variable's name.
var nameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Handler writes secrets.
type Handler struct {
	paths sysdep.Paths
	run   sysdep.Runner
	log   *slog.Logger
	uid   int
	gid   int
}

// New builds the handler. uid and gid own the secret files; a real guest
// passes dev's.
func New(p sysdep.Paths, run sysdep.Runner, log *slog.Logger) *Handler {
	return &Handler{paths: p, run: run, log: log, uid: sysdep.DevUID, gid: sysdep.DevGID}
}

// Write replaces the guest's named secrets with list. The list is the whole
// set: a name that was there and is not in the list is removed, which is what
// makes `repose secrets rm` reach the guest. Reserved names are never removed
// this way, because they arrive on the create path, not the secrets path.
//
// Validation happens before any write, so a bad batch changes nothing.
func (h *Handler) Write(ctx context.Context, list []*guestdv1.Secret) error {
	seen := make(map[string]bool, len(list))
	for _, s := range list {
		name := s.GetName()
		switch {
		case name == "":
			return sysdep.Invalid("write secrets: a secret has no name")
		case len(name) > MaxNameBytes:
			return sysdep.Invalid("write secrets: a secret name is longer than %d bytes", MaxNameBytes)
		case seen[name]:
			return sysdep.Invalid("write secrets: %s appears twice", name)
		}
		if _, ok := reserved[name]; !ok {
			if !nameRe.MatchString(name) {
				return sysdep.Invalid("write secrets: %s is not a valid environment variable name", name)
			}
			if bytes.IndexByte(s.GetValue(), 0) >= 0 {
				return sysdep.Invalid("write secrets: the value of %s contains a NUL byte", name)
			}
		}
		if len(s.GetValue()) > MaxValueBytes {
			return sysdep.Invalid("write secrets: the value of %s is larger than %d bytes", name, MaxValueBytes)
		}
		seen[name] = true
	}

	if err := h.ensureDirs(); err != nil {
		return err
	}

	named := 0
	sshMaterial := false
	for _, s := range list {
		if spec, ok := reserved[s.GetName()]; ok {
			path := filepath.Join(h.paths.RunDir(), spec.file)
			if err := sysdep.WriteFileAtomic(path, s.GetValue(), spec.mode, 0, 0); err != nil {
				return sysdep.Errf(sysdep.CodeInternal, "write sshd material: %w", err)
			}
			sshMaterial = true
			continue
		}
		path := filepath.Join(h.paths.SecretsDir(), s.GetName())
		if err := sysdep.WriteFileAtomic(path, s.GetValue(), 0o400, h.uid, h.gid); err != nil {
			return sysdep.Errf(sysdep.CodeInternal, "write secret: %w", err)
		}
		named++
	}

	removed, err := h.removeStale(seen)
	if err != nil {
		return err
	}
	if err := h.writeEnv(list); err != nil {
		return err
	}
	if sshMaterial {
		if err := h.reloadSSHD(ctx); err != nil {
			return err
		}
	}

	h.log.Info("secrets written",
		"event", "write_secrets", "count", named, "removed", removed, "ssh_material", sshMaterial)
	return nil
}

func (h *Handler) ensureDirs() error {
	if err := os.MkdirAll(h.paths.RunDir(), 0o755); err != nil {
		return sysdep.Errf(sysdep.CodeInternal, "create run directory: %w", err)
	}
	if err := os.MkdirAll(h.paths.SecretsDir(), 0o700); err != nil {
		return sysdep.Errf(sysdep.CodeInternal, "create secrets directory: %w", err)
	}
	if err := os.Chown(h.paths.SecretsDir(), h.uid, h.gid); err != nil && !os.IsPermission(err) {
		return sysdep.Errf(sysdep.CodeInternal, "chown secrets directory: %w", err)
	}
	return nil
}

// removeStale deletes secret files whose names are no longer in the set.
func (h *Handler) removeStale(keep map[string]bool) (int, error) {
	entries, err := os.ReadDir(h.paths.SecretsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, sysdep.Errf(sysdep.CodeInternal, "list secrets directory: %w", err)
	}
	removed := 0
	for _, e := range entries {
		if e.IsDir() || keep[e.Name()] || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if err := os.Remove(filepath.Join(h.paths.SecretsDir(), e.Name())); err != nil {
			return removed, sysdep.Errf(sysdep.CodeInternal, "remove withdrawn secret: %w", err)
		}
		removed++
	}
	return removed, nil
}

// writeEnv rewrites /run/repose/secrets.env from the non-reserved secrets, in
// name order so the file does not churn.
func (h *Handler) writeEnv(list []*guestdv1.Secret) error {
	type kv struct{ name, value string }
	rows := make([]kv, 0, len(list))
	for _, s := range list {
		if _, ok := reserved[s.GetName()]; ok {
			continue
		}
		rows = append(rows, kv{s.GetName(), string(s.GetValue())})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })

	var buf bytes.Buffer
	buf.WriteString("# Written by guestd. Sourced by login shells; do not edit.\n")
	for _, r := range rows {
		fmt.Fprintf(&buf, "export %s=%s\n", r.name, shellQuote(r.value))
	}
	if err := sysdep.WriteFileAtomic(h.paths.SecretsEnv(), buf.Bytes(), 0o400, h.uid, h.gid); err != nil {
		return sysdep.Errf(sysdep.CodeInternal, "write secrets env file: %w", err)
	}
	return nil
}

// shellQuote single-quotes a value, escaping an embedded quote as '\” so a
// value containing quotes or newlines survives being sourced.
func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

func (h *Handler) reloadSSHD(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	res, err := h.run.Run(ctx, sysdep.RunSpec{
		Argv:      []string{"systemctl", "reload-or-restart", "sshd.service"},
		MaxOutput: 4 << 10,
		Env:       sysdep.DevEnv(h.paths, "root"),
	})
	if err != nil {
		return sysdep.Errf(sysdep.CodeInternal, "reload sshd after writing host key: %w", err)
	}
	if res.ExitCode != 0 {
		return sysdep.Errf(sysdep.CodeInternal, "reload sshd after writing host key: systemctl exited %d", res.ExitCode)
	}
	return nil
}

// IsReserved reports whether name is one of the sshd material names.
func IsReserved(name string) bool {
	_, ok := reserved[name]
	return ok
}
