// Package secrets is the envelope encryption behind named secrets
// (docs/features/secrets.md, 05-control-plane-api.md §5.6): values are
// AES-256-GCM under a per-user data key, the data key is wrapped by a Key
// Vault key, and both ciphertext and wrapped key live on every secrets
// row. Values are decrypted in exactly one place, DecryptForGuest, whose
// only callers are the hostd command builders.
package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/heracraft/repose/internal/db"
)

// KeyVault wraps and unwraps data keys. The real one is Azure Key Vault;
// tests use internal/fakes/kv.
type KeyVault interface {
	Wrap(ctx context.Context, dek []byte) (wrapped []byte, keyVersion string, err error)
	Unwrap(ctx context.Context, wrapped []byte, keyVersion string) (dek []byte, err error)
	CurrentVersion(ctx context.Context) (string, error)
}

// ErrKeyServiceUnavailable is surfaced as `internal: key service
// unavailable`; guest starts retry on it.
var ErrKeyServiceUnavailable = errors.New("key service unavailable")

// ErrInvalidName is returned for names outside the contract.
var ErrInvalidName = errors.New("invalid secret name")

// ErrTooLarge is returned for values over MaxValueBytes.
var ErrTooLarge = errors.New("secret value exceeds 64 KB")

// MaxValueBytes is the value cap from docs/interfaces/api.md.
const MaxValueBytes = 64 << 10

// PlatformProjectID is the pseudo-project that owns the CA material.
const PlatformProjectID = "00000000-0000-7000-8000-000000000000"

// PlatformUserID is the pseudo-user behind it.
const PlatformUserID = "00000000-0000-7000-8000-000000000000"

var nameRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

// reserved names are the guest sshd material delivered by CreateGuest
// (DECISIONS I-10); they are refused on PUT.
var reserved = map[string]bool{
	"ssh_host_ed25519_key":          true,
	"ssh_host_ed25519_key-cert.pub": true,
	"user_ca.pub":                   true,
}

// ValidName reports whether a user-facing name is acceptable.
func ValidName(name string) bool {
	return nameRe.MatchString(name) && !reserved[name]
}

// IsReserved reports whether the name is one of the sshd material names.
func IsReserved(name string) bool { return reserved[name] }

// Store is the secrets service.
type Store struct {
	pool *db.Pool
	kv   KeyVault

	mu    sync.Mutex
	cache map[string]cachedDEK
	ttl   time.Duration
}

type cachedDEK struct {
	dek     []byte
	expires time.Time
}

// New builds a store.
func New(pool *db.Pool, kv KeyVault) *Store {
	return &Store{pool: pool, kv: kv, cache: map[string]cachedDEK{}, ttl: 10 * time.Minute}
}

// Meta is what GET /secrets returns: never a value.
type Meta struct {
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NamedValue is a decrypted secret, for a hostd command only.
type NamedValue struct {
	Name  string
	Value []byte
}

func cacheKey(wrapped []byte, version string) string {
	h := sha256.Sum256(wrapped)
	return version + ":" + hex.EncodeToString(h[:8])
}

func kvErr(err error) error {
	return fmt.Errorf("%w: %v", ErrKeyServiceUnavailable, err)
}

func (s *Store) unwrap(ctx context.Context, wrapped []byte, version string) ([]byte, error) {
	k := cacheKey(wrapped, version)
	s.mu.Lock()
	if c, ok := s.cache[k]; ok && time.Now().Before(c.expires) {
		s.mu.Unlock()
		return c.dek, nil
	}
	s.mu.Unlock()
	dek, err := s.kv.Unwrap(ctx, wrapped, version)
	if err != nil {
		return nil, kvErr(err)
	}
	s.mu.Lock()
	s.cache[k] = cachedDEK{dek: dek, expires: time.Now().Add(s.ttl)}
	s.mu.Unlock()
	return dek, nil
}

// userDEK returns the user's wrapped DEK from any existing row, or wraps
// a fresh one. Two first-time PUTs racing can create two DEKs for a user;
// every row carries its own wrapped key, so both stay decryptable.
func (s *Store) userDEK(ctx context.Context, tx pgx.Tx, userID string) (dek, wrapped []byte, version string, err error) {
	err = tx.QueryRow(ctx, `select s.dek_wrapped, s.kv_key_version from secrets s join projects p on p.id = s.project_id where p.user_id = $1 order by s.created_at limit 1`, userID).Scan(&wrapped, &version)
	if err == nil {
		dek, err = s.unwrap(ctx, wrapped, version)
		return dek, wrapped, version, err
	}
	if !db.IsNoRows(err) {
		return nil, nil, "", err
	}
	dek = make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return nil, nil, "", err
	}
	wrapped, version, err = s.kv.Wrap(ctx, dek)
	if err != nil {
		return nil, nil, "", kvErr(err)
	}
	s.mu.Lock()
	s.cache[cacheKey(wrapped, version)] = cachedDEK{dek: dek, expires: time.Now().Add(s.ttl)}
	s.mu.Unlock()
	return dek, wrapped, version, nil
}

// Seal encrypts value under dek with the name as additional authenticated
// data, so a ciphertext cannot be moved between names.
func Seal(dek []byte, name string, value []byte) ([]byte, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append(nonce, gcm.Seal(nil, nonce, value, []byte(name))...), nil
}

// Open reverses Seal.
func Open(dek []byte, name string, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	return gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], []byte(name))
}

// Put stores or replaces a secret.
func (s *Store) Put(ctx context.Context, userID, projectID, name string, value []byte) error {
	if !nameRe.MatchString(name) && !s.isPlatform(projectID) {
		return ErrInvalidName
	}
	if reserved[name] {
		return ErrInvalidName
	}
	if len(value) > MaxValueBytes && !s.isPlatform(projectID) {
		return ErrTooLarge
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		dek, wrapped, version, err := s.userDEK(ctx, tx, userID)
		if err != nil {
			return err
		}
		ct, err := Seal(dek, name, value)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `insert into secrets (id, project_id, name, ciphertext, dek_wrapped, kv_key_version) values ($1, $2, $3, $4, $5, $6)
			on conflict (project_id, name) do update set ciphertext = excluded.ciphertext, dek_wrapped = excluded.dek_wrapped, kv_key_version = excluded.kv_key_version`,
			uuid.Must(uuid.NewV7()), projectID, name, ct, wrapped, version)
		return err
	})
}

func (s *Store) isPlatform(projectID string) bool { return projectID == PlatformProjectID }

// PutReserved stores the guest sshd material (DECISIONS I-10, I-35) under
// one of the reserved names; user routes never reach it.
func (s *Store) PutReserved(ctx context.Context, userID, projectID, name string, value []byte) error {
	if !reserved[name] {
		return ErrInvalidName
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		dek, wrapped, version, err := s.userDEK(ctx, tx, userID)
		if err != nil {
			return err
		}
		ct, err := Seal(dek, name, value)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `insert into secrets (id, project_id, name, ciphertext, dek_wrapped, kv_key_version) values ($1, $2, $3, $4, $5, $6)
			on conflict (project_id, name) do update set ciphertext = excluded.ciphertext, dek_wrapped = excluded.dek_wrapped, kv_key_version = excluded.kv_key_version`,
			uuid.Must(uuid.NewV7()), projectID, name, ct, wrapped, version)
		return err
	})
}

// Delete removes a secret; missing is not an error.
func (s *Store) Delete(ctx context.Context, projectID, name string) (bool, error) {
	tag, err := s.pool.Exec(ctx, "delete from secrets where project_id = $1 and name = $2", projectID, name)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// List returns names and timestamps of the user's secrets; the reserved
// sshd material is not listed.
func (s *Store) List(ctx context.Context, projectID string) ([]Meta, error) {
	rows, err := s.pool.Query(ctx, "select name, created_at, updated_at from secrets where project_id = $1 and name not in ('ssh_host_ed25519_key','ssh_host_ed25519_key-cert.pub','user_ca.pub') order by name", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Meta{}
	for rows.Next() {
		var m Meta
		if err := rows.Scan(&m.Name, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DecryptForGuest returns every secret of a project in plaintext. It
// exists for the hostd command builders (CreateGuest, StartGuest,
// Restore, UpdateSecrets) and for the platform's own CA material; nothing
// else may call it.
func (s *Store) DecryptForGuest(ctx context.Context, projectID string) ([]NamedValue, error) {
	rows, err := s.pool.Query(ctx, "select name, ciphertext, dek_wrapped, kv_key_version from secrets where project_id = $1 order by name", projectID)
	if err != nil {
		return nil, err
	}
	type row struct {
		name, version string
		ct, wrapped   []byte
	}
	var rs []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.name, &r.ct, &r.wrapped, &r.version); err != nil {
			rows.Close()
			return nil, err
		}
		rs = append(rs, r)
	}
	rows.Close()
	out := []NamedValue{}
	for _, r := range rs {
		dek, err := s.unwrap(ctx, r.wrapped, r.version)
		if err != nil {
			return nil, err
		}
		v, err := Open(dek, r.name, r.ct)
		if err != nil {
			return nil, fmt.Errorf("secret %s: %w", r.name, err)
		}
		out = append(out, NamedValue{Name: r.name, Value: v})
	}
	return out, nil
}

// Rewrap re-wraps every row's DEK under the Key Vault key's current
// version without touching ciphertext, and returns how many rows changed.
func (s *Store) Rewrap(ctx context.Context) (int, error) {
	current, err := s.kv.CurrentVersion(ctx)
	if err != nil {
		return 0, kvErr(err)
	}
	rows, err := s.pool.Query(ctx, "select id, dek_wrapped, kv_key_version from secrets where kv_key_version <> $1", current)
	if err != nil {
		return 0, err
	}
	type row struct {
		id      uuid.UUID
		wrapped []byte
		version string
	}
	var rs []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.wrapped, &r.version); err != nil {
			rows.Close()
			return 0, err
		}
		rs = append(rs, r)
	}
	rows.Close()
	n := 0
	rewrapped := map[string][]byte{}
	for _, r := range rs {
		k := cacheKey(r.wrapped, r.version)
		nw, ok := rewrapped[k]
		if !ok {
			dek, err := s.unwrap(ctx, r.wrapped, r.version)
			if err != nil {
				return n, err
			}
			nw, _, err = s.kv.Wrap(ctx, dek)
			if err != nil {
				return n, kvErr(err)
			}
			rewrapped[k] = nw
			s.mu.Lock()
			s.cache[cacheKey(nw, current)] = cachedDEK{dek: dek, expires: time.Now().Add(s.ttl)}
			s.mu.Unlock()
		}
		if _, err := s.pool.Exec(ctx, "update secrets set dek_wrapped = $1, kv_key_version = $2 where id = $3 and kv_key_version = $4", nw, current, r.id, r.version); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// PutPlatform stores platform material (CA keys) under the pseudo-project.
func (s *Store) PutPlatform(ctx context.Context, name string, value []byte) error {
	return s.Put(ctx, PlatformUserID, PlatformProjectID, name, value)
}

// GetPlatform reads one platform value; db.ErrNotFound when absent.
func (s *Store) GetPlatform(ctx context.Context, name string) ([]byte, error) {
	vals, err := s.DecryptForGuest(ctx, PlatformProjectID)
	if err != nil {
		return nil, err
	}
	for _, v := range vals {
		if v.Name == name {
			return v.Value, nil
		}
	}
	return nil, db.ErrNotFound
}
