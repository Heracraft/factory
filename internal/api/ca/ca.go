// Package ca is the api's SSH certificate authority (docs/interfaces/
// ssh-gateway.md, 05-control-plane-api.md §5.5): two ed25519 keys kept
// encrypted in the secrets table under the platform pseudo-project and
// held in memory after start; user certificates with project principals,
// the gateway's 5-minute certificates (I-1), guest host keys and
// certificates (I-3), revocation, and the X.509 host CA behind host mTLS.
package ca

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

	"github.com/heracraft/repose/internal/api/pki"
	"github.com/heracraft/repose/internal/api/secrets"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/ca/sshca"
	"github.com/heracraft/repose/internal/db"
)

// Names of the platform secrets that hold the CA material.
const (
	SecretUserCA      = "SSH_USER_CA"
	SecretHostCA      = "SSH_HOST_CA"
	SecretX509CACert  = "X509_HOST_CA_CERT"
	SecretX509CAKey   = "X509_HOST_CA_KEY"
	GuestHostCertTTL  = 10 * 365 * 24 * time.Hour
	GatewayHostCertTL = 5 * 365 * 24 * time.Hour
)

// ErrNotInitialised is returned by Load before `repose-admin ca init`.
var ErrNotInitialised = errors.New("ca: not initialised; run `repose-admin ca init`")

// ErrNotOwner is returned when a project id in a cert request is not the
// user's.
var ErrNotOwner = errors.New("project not found")

// CA is the loaded authority.
type CA struct {
	pool *db.Pool
	user *sshca.Key
	host *sshca.Key
	x509 *pki.CA
}

// Init generates every CA key and stores it, refusing to overwrite.
func Init(ctx context.Context, sec *secrets.Store) error {
	if _, err := sec.GetPlatform(ctx, SecretUserCA); err == nil {
		return errors.New("ca: already initialised")
	} else if !errors.Is(err, db.ErrNotFound) {
		return err
	}
	return generate(ctx, sec, true, true)
}

func generate(ctx context.Context, sec *secrets.Store, user, host bool) error {
	if user {
		_, pemBytes, err := sshca.Generate("repose user ca")
		if err != nil {
			return err
		}
		if err := sec.PutPlatform(ctx, SecretUserCA, pemBytes); err != nil {
			return err
		}
	}
	if host {
		_, pemBytes, err := sshca.Generate("repose host ca")
		if err != nil {
			return err
		}
		if err := sec.PutPlatform(ctx, SecretHostCA, pemBytes); err != nil {
			return err
		}
		x, err := pki.Generate("repose host ca")
		if err != nil {
			return err
		}
		keyPEM, err := x.KeyPEM()
		if err != nil {
			return err
		}
		if err := sec.PutPlatform(ctx, SecretX509CACert, x.CertPEM); err != nil {
			return err
		}
		if err := sec.PutPlatform(ctx, SecretX509CAKey, keyPEM); err != nil {
			return err
		}
	}
	return nil
}

// Rotate replaces the User CA, the Host CA, or both. Certificates signed
// by the old User CA expire within 12 hours; guests get new host
// certificates on their next start.
func Rotate(ctx context.Context, sec *secrets.Store, user, host bool) error {
	if _, err := sec.GetPlatform(ctx, SecretUserCA); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ErrNotInitialised
		}
		return err
	}
	return generate(ctx, sec, user, host)
}

// Load reads the CA material.
func Load(ctx context.Context, pool *db.Pool, sec *secrets.Store) (*CA, error) {
	get := func(name string) ([]byte, error) {
		b, err := sec.GetPlatform(ctx, name)
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrNotInitialised
		}
		return b, err
	}
	up, err := get(SecretUserCA)
	if err != nil {
		return nil, err
	}
	hp, err := get(SecretHostCA)
	if err != nil {
		return nil, err
	}
	xc, err := get(SecretX509CACert)
	if err != nil {
		return nil, err
	}
	xk, err := get(SecretX509CAKey)
	if err != nil {
		return nil, err
	}
	user, err := sshca.Load(up)
	if err != nil {
		return nil, err
	}
	host, err := sshca.Load(hp)
	if err != nil {
		return nil, err
	}
	x, err := pki.Load(xc, xk)
	if err != nil {
		return nil, err
	}
	return &CA{pool: pool, user: user, host: host, x509: x}, nil
}

// NewInMemory builds a CA from generated keys for tests and dev mode.
func NewInMemory(pool *db.Pool) (*CA, error) {
	user, _, err := sshca.Generate("repose user ca (memory)")
	if err != nil {
		return nil, err
	}
	host, _, err := sshca.Generate("repose host ca (memory)")
	if err != nil {
		return nil, err
	}
	x, err := pki.Generate("repose host ca (memory)")
	if err != nil {
		return nil, err
	}
	return &CA{pool: pool, user: user, host: host, x509: x}, nil
}

// UserCAPub is the User CA public key line.
func (c *CA) UserCAPub() string { return c.user.PublicLine() }

// HostCAPub is the Host CA public key line.
func (c *CA) HostCAPub() string { return c.host.PublicLine() }

// X509 is the host mTLS authority.
func (c *CA) X509() *pki.CA { return c.x509 }

// Issued is a minted certificate.
type Issued struct {
	Certificate string
	Serial      int64
	ExpiresAt   time.Time
}

func nextSerial(ctx context.Context, q store.Querier) (int64, error) {
	var s int64
	err := q.QueryRow(ctx, "select nextval('certificates_serial')").Scan(&s)
	return s, err
}

// IssueUserCert mints a user certificate for the user's key with the
// project ids as principals, after checking every project is the user's.
func (c *CA) IssueUserCert(ctx context.Context, user *store.User, pub ssh.PublicKey, projectIDs []uuid.UUID) (*Issued, error) {
	if len(projectIDs) == 0 {
		return nil, errors.New("project_ids required")
	}
	var out *Issued
	err := db.InTx(ctx, c.pool, func(tx db.Tx) error {
		principals := make([]string, 0, len(projectIDs))
		for _, id := range projectIDs {
			if _, err := store.GetUserProject(ctx, tx, user.ID, id); err != nil {
				if errors.Is(err, db.ErrNotFound) {
					return fmt.Errorf("%w: %s", ErrNotOwner, id)
				}
				return err
			}
			principals = append(principals, id.String())
		}
		serial, err := nextSerial(ctx, tx)
		if err != nil {
			return err
		}
		now := time.Now()
		expires := now.Add(sshca.UserCertTTL)
		keyID := user.ID.String() + ":" + user.Handle
		cert, err := c.user.SignUser(sshca.UserCert{PublicKey: pub, KeyID: keyID, Principals: principals, Serial: uint64(serial), ValidAfter: now.Add(-time.Minute), ValidBefore: expires})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `insert into certificates (serial, user_id, project_ids, public_key_fp, key_id, kind, expires_at) values ($1, $2, $3, $4, $5, 'user', $6)`,
			serial, user.ID, projectIDs, sshca.Fingerprint(pub), keyID, expires); err != nil {
			return err
		}
		if _, err := store.Audit(ctx, tx, "user:"+user.ID.String(), "cert_issue", fmt.Sprint(serial), map[string]any{"projects": len(projectIDs), "kind": "user"}); err != nil {
			return err
		}
		out = &Issued{Certificate: sshca.Marshal(cert), Serial: serial, ExpiresAt: expires}
		return nil
	})
	return out, err
}

// IssueGatewayCert mints the gateway's 5-minute certificate for one
// project (I-1); key_id is suffixed :via-gateway.
func (c *CA) IssueGatewayCert(ctx context.Context, pub ssh.PublicKey, project *store.Project) (*Issued, error) {
	var out *Issued
	err := db.InTx(ctx, c.pool, func(tx db.Tx) error {
		user, err := store.GetUser(ctx, tx, project.UserID)
		if err != nil {
			return err
		}
		serial, err := nextSerial(ctx, tx)
		if err != nil {
			return err
		}
		now := time.Now()
		expires := now.Add(sshca.GatewayCertTTL)
		keyID := user.ID.String() + ":" + user.Handle + ":via-gateway"
		cert, err := c.user.SignUser(sshca.UserCert{PublicKey: pub, KeyID: keyID, Principals: []string{project.ID.String()}, Serial: uint64(serial), ValidAfter: now.Add(-time.Minute), ValidBefore: expires})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `insert into certificates (serial, user_id, project_ids, public_key_fp, key_id, kind, expires_at) values ($1, $2, $3, $4, $5, 'gateway', $6)`,
			serial, user.ID, []uuid.UUID{project.ID}, sshca.Fingerprint(pub), keyID, expires); err != nil {
			return err
		}
		out = &Issued{Certificate: sshca.Marshal(cert), Serial: serial, ExpiresAt: expires}
		return nil
	})
	return out, err
}

// GuestHostKey is the sshd material CreateGuest carries (I-3).
type GuestHostKey struct {
	PrivateKeyPEM []byte
	Certificate   string
}

// NewGuestHostKey generates a guest's sshd host key and signs its
// certificate with principals guest_ip and <slug>.<handle>.
func (c *CA) NewGuestHostKey(ctx context.Context, principals []string) (*GuestHostKey, error) {
	priv, pub, err := sshca.GenerateHostKey("repose guest host key")
	if err != nil {
		return nil, err
	}
	serial, err := nextSerial(ctx, c.pool)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	cert, err := c.host.SignHost(sshca.HostCert{PublicKey: pub, KeyID: "guest:" + principals[len(principals)-1], Principals: principals, Serial: uint64(serial), ValidAfter: now.Add(-time.Minute), ValidBefore: now.Add(GuestHostCertTTL)})
	if err != nil {
		return nil, err
	}
	return &GuestHostKey{PrivateKeyPEM: priv, Certificate: sshca.Marshal(cert)}, nil
}

// SignHostCert signs a host certificate for an existing public key (the
// gateway's host key, `repose-admin ca sign-host`).
func (c *CA) SignHostCert(ctx context.Context, pub ssh.PublicKey, principals []string, ttl time.Duration) (string, error) {
	serial, err := nextSerial(ctx, c.pool)
	if err != nil {
		return "", err
	}
	now := time.Now()
	cert, err := c.host.SignHost(sshca.HostCert{PublicKey: pub, KeyID: "host:" + principals[0], Principals: principals, Serial: uint64(serial), ValidAfter: now.Add(-time.Minute), ValidBefore: now.Add(ttl)})
	if err != nil {
		return "", err
	}
	return sshca.Marshal(cert), nil
}

// SignOperatorCert signs an operator user certificate with the Host CA
// (the edge's operator sshd trusts it; RUNBOOK "operator-cert").
func (c *CA) SignOperatorCert(ctx context.Context, pub ssh.PublicKey, name string, ttl time.Duration) (string, error) {
	serial, err := nextSerial(ctx, c.pool)
	if err != nil {
		return "", err
	}
	now := time.Now()
	cert, err := c.host.SignUser(sshca.UserCert{PublicKey: pub, KeyID: "operator:" + name, Principals: []string{"root", "operator"}, Serial: uint64(serial), ValidAfter: now.Add(-time.Minute), ValidBefore: now.Add(ttl)})
	if err != nil {
		return "", err
	}
	if _, err := store.Audit(ctx, c.pool, "admin", "operator_cert", name, map[string]any{"serial": serial}); err != nil {
		return "", err
	}
	return sshca.Marshal(cert), nil
}

// Revoke marks one of the user's certificates revoked; a serial that is
// not theirs is not found.
func (c *CA) Revoke(ctx context.Context, userID uuid.UUID, serial int64) error {
	tag, err := c.pool.Exec(ctx, "update certificates set revoked_at = now() where serial = $1 and user_id = $2 and revoked_at is null", serial, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := c.pool.QueryRow(ctx, "select exists(select 1 from certificates where serial = $1 and user_id = $2)", serial, userID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return db.ErrNotFound
		}
	}
	_, err = store.Audit(ctx, c.pool, "user:"+userID.String(), "cert_revoke", fmt.Sprint(serial), nil)
	return err
}

// RevokeAll revokes every live certificate of a user and returns how many.
func (c *CA) RevokeAll(ctx context.Context, userID uuid.UUID, actor string) (int64, error) {
	tag, err := c.pool.Exec(ctx, "update certificates set revoked_at = now() where user_id = $1 and revoked_at is null and expires_at > now()", userID)
	if err != nil {
		return 0, err
	}
	if _, err := store.Audit(ctx, c.pool, actor, "cert_revoke_all", userID.String(), map[string]any{"count": tag.RowsAffected()}); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// RevokedSince lists serials revoked after since (the gateway's poll);
// the zero time lists every revoked certificate that has not expired.
func (c *CA) RevokedSince(ctx context.Context, since time.Time) ([]int64, error) {
	rows, err := c.pool.Query(ctx, "select serial from certificates where revoked_at is not null and revoked_at > $1 and expires_at > now() - interval '1 hour' order by serial", since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var s int64
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Prune deletes certificate rows expired for longer than age (the
// certcleanup loop) and returns how many.
func (c *CA) Prune(ctx context.Context, age time.Duration) (int64, error) {
	tag, err := c.pool.Exec(ctx, "delete from certificates where expires_at < now() - $1::interval", age.String())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
