// Package settings is a key/value store for application configuration edited
// from the UI (external service URLs, tokens and API keys). The values may be
// secret; the SQLite file is owner-only on disk.
package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Well-known setting keys.
const (
	// Jellyfin replaced Plex on this HTPC. KeyJellyfinDeviceID identifies this
	// install to the server across restarts, so its device list keeps one
	// Holocron entry; the user id is needed for any per-user query.
	KeyJellyfinURL      = "jellyfin.url"
	KeyJellyfinToken    = "jellyfin.token"
	KeyJellyfinUserID   = "jellyfin.user_id"
	KeyJellyfinUser     = "jellyfin.user"
	KeyJellyfinAdmin    = "jellyfin.is_admin"
	KeyJellyfinDeviceID = "jellyfin.device_id"
	KeyQbitURL          = "qbittorrent.url"
	KeyQbitUser         = "qbittorrent.username"
	KeyQbitPass         = "qbittorrent.password"
	// KeyAPITokenHash holds the SHA-256 digest of the JSON API bearer token
	// (never the token itself). See internal/apitoken.
	//#nosec G101 -- the name of a settings key, not a credential
	KeyAPITokenHash = "api.token_hash"
)

// Store reads and writes settings.
type Store struct {
	db *sql.DB
	// managed holds values the server provides and the UI must not edit, such
	// as API keys systemd hands over with LoadCredential. They are consulted
	// before the database and never written to it. See credentials.go.
	managed map[string]string
	// defaults fill in a setting nobody saved, such as a service address that
	// is loopback on Ginebra. A saved value always wins.
	defaults map[string]string
}

// NewStore creates a Store.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Get returns the value for key and whether it is set.
func (s *Store) Get(ctx context.Context, key string) (string, bool, error) {
	if v, ok := s.managed[key]; ok {
		return v, true, nil
	}
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		if d, ok := s.defaults[key]; ok {
			return d, true, nil
		}
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get setting %q: %w", key, err)
	}
	return v, true, nil
}

// GetDefault returns the value for key, or fallback if unset.
func (s *Store) GetDefault(ctx context.Context, key, fallback string) string {
	if v, ok, err := s.Get(ctx, key); err == nil && ok {
		return v
	}
	return fallback
}

// Set stores value under key (upsert). An empty value deletes the key.
func (s *Store) Set(ctx context.Context, key, value string) error {
	if _, ok := s.managed[key]; ok {
		return fmt.Errorf("set setting %q: %w", key, ErrManaged)
	}
	if value == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
		if err != nil {
			return fmt.Errorf("delete setting %q: %w", key, err)
		}
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value)
	if err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}
	return nil
}
