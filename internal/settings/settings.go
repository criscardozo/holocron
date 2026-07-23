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
	KeyPlexURL           = "plex.url"
	KeyPlexToken         = "plex.token"
	KeyOpenSubtitlesKey  = "opensubtitles.api_key"
	KeyOpenSubtitlesUser = "opensubtitles.username"
	KeyOpenSubtitlesPass = "opensubtitles.password"
	KeyQbitURL           = "qbittorrent.url"
	KeyQbitUser          = "qbittorrent.username"
	KeyQbitPass          = "qbittorrent.password"
)

// Store reads and writes settings.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Get returns the value for key and whether it is set.
func (s *Store) Get(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
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
