package notify

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/db"
)

// UnsubKeyName is the platform secret holding the unsubscribe HMAC key,
// stored the same way as the CA material (internal/api/ca): ciphertext in
// the secrets table under the platform pseudo-project, never a fourth home.
const UnsubKeyName = "NOTIFY_UNSUB_KEY"

// PlatformSecrets is the slice of secrets.Store an Unsubscriber needs.
type PlatformSecrets interface {
	GetPlatform(ctx context.Context, name string) ([]byte, error)
	PutPlatform(ctx context.Context, name string, value []byte) error
}

// Unsubscriber signs and verifies the one-click unsubscribe link
// (13-notifications.md §5.6): the token names the user and is checked
// without a database round trip, so the link works even if the click
// arrives with no session and no bearer token.
type Unsubscriber struct{ key []byte }

// LoadOrCreateUnsubscriber reads the signing key, generating and storing
// one the first time the api starts. An operator step here (mirroring
// `repose-admin ca init`) would leave the very first account's unsubscribe
// link broken until someone remembered to run it, so this one key
// provisions itself.
func LoadOrCreateUnsubscriber(ctx context.Context, sec PlatformSecrets) (*Unsubscriber, error) {
	key, err := sec.GetPlatform(ctx, UnsubKeyName)
	if err == nil {
		return &Unsubscriber{key: key}, nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}
	key = make([]byte, 32)
	if _, rerr := rand.Read(key); rerr != nil {
		return nil, rerr
	}
	if perr := sec.PutPlatform(ctx, UnsubKeyName, key); perr != nil {
		return nil, perr
	}
	return &Unsubscriber{key: key}, nil
}

// Sign returns the token embedded in a `GET /notify/unsubscribe?token=` link.
func (u *Unsubscriber) Sign(userID uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString(userID[:]) + "." + base64.RawURLEncoding.EncodeToString(u.mac(userID))
}

// Verify recovers the user id from a token, or an error if it is malformed
// or was not signed by this key.
func (u *Unsubscriber) Verify(token string) (uuid.UUID, error) {
	idPart, sigPart, ok := strings.Cut(token, ".")
	if !ok {
		return uuid.Nil, errors.New("unsubscribe: malformed token")
	}
	idBytes, err := base64.RawURLEncoding.DecodeString(idPart)
	if err != nil {
		return uuid.Nil, errors.New("unsubscribe: malformed token")
	}
	id, err := uuid.FromBytes(idBytes)
	if err != nil {
		return uuid.Nil, errors.New("unsubscribe: malformed token")
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigPart)
	if err != nil {
		return uuid.Nil, errors.New("unsubscribe: malformed token")
	}
	if !hmac.Equal(sig, u.mac(id)) {
		return uuid.Nil, errors.New("unsubscribe: invalid token")
	}
	return id, nil
}

func (u *Unsubscriber) mac(id uuid.UUID) []byte {
	h := hmac.New(sha256.New, u.key)
	h.Write(id[:])
	return h.Sum(nil)
}

// URL builds the unsubscribe link an email template embeds. base is the
// api's own public origin (`API_RESOURCE`), not the dashboard's: the route
// is served by the api, not the SPA, at the same /v1 prefix as every other
// route (docs/interfaces/api.md).
func (u *Unsubscriber) URL(base string, userID uuid.UUID) string {
	return fmt.Sprintf("%s/v1/notify/unsubscribe?token=%s", strings.TrimRight(base, "/"), u.Sign(userID))
}
