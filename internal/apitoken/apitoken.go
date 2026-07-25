// Package apitoken issues and verifies the bearer token that guards the JSON
// API used by the iOS app. The web UI is deliberately unauthenticated (trusted
// LAN); the API is not, because a phone leaves the LAN.
//
// Only a SHA-256 digest of the token is stored: the plaintext is shown once at
// generation time and never again, so a copy of the database does not hand over
// API access.
package apitoken

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/cristian/holocron/internal/settings"
)

// tokenBytes is the entropy of a generated token (256 bits).
const tokenBytes = 32

// ErrNoToken means no API token has been generated yet, so the API is closed.
var ErrNoToken = errors.New("no api token configured")

// Store issues and verifies API tokens on top of the settings store.
type Store struct {
	settings *settings.Store
}

// NewStore creates a Store.
func NewStore(st *settings.Store) *Store { return &Store{settings: st} }

// Generate creates a new token, replaces any previous one, and returns the
// plaintext. This is the only time the plaintext exists: only its digest is
// persisted.
func (s *Store) Generate(ctx context.Context) (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate api token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := s.settings.Set(ctx, settings.KeyAPITokenHash, digest(token)); err != nil {
		return "", err
	}
	return token, nil
}

// Revoke removes the stored token, closing the API.
func (s *Store) Revoke(ctx context.Context) error {
	return s.settings.Set(ctx, settings.KeyAPITokenHash, "")
}

// Configured reports whether a token has been generated.
func (s *Store) Configured(ctx context.Context) bool {
	return s.settings.GetDefault(ctx, settings.KeyAPITokenHash, "") != ""
}

// Verify reports whether presented matches the stored token. The comparison is
// constant-time so a caller cannot learn the token by timing its guesses.
func (s *Store) Verify(ctx context.Context, presented string) error {
	stored := s.settings.GetDefault(ctx, settings.KeyAPITokenHash, "")
	if stored == "" {
		return ErrNoToken
	}
	if subtle.ConstantTimeCompare([]byte(digest(presented)), []byte(stored)) != 1 {
		return errors.New("invalid api token")
	}
	return nil
}

// BearerToken extracts the token from an Authorization header value.
func BearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

func digest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
