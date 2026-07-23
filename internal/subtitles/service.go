// Package subtitles finds media without Spanish subtitles and downloads them
// from OpenSubtitles into the media folder.
package subtitles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cristian/holocron/internal/opensubtitles"
	"github.com/cristian/holocron/internal/settings"
)

const language = "es"

// ErrNotConfigured means the OpenSubtitles API key has not been set.
var ErrNotConfigured = errors.New("opensubtitles is not configured")

// Service orchestrates subtitle discovery and download.
type Service struct {
	db       *sql.DB
	settings *settings.Store
}

// NewService creates a Service.
func NewService(db *sql.DB, st *settings.Store) *Service {
	return &Service{db: db, settings: st}
}

// MissingItem is a media entry lacking a Spanish subtitle.
type MissingItem struct {
	Path  string
	Type  string
	Title string
	Year  int
}

// Configured reports whether an OpenSubtitles API key is set.
func (s *Service) Configured(ctx context.Context) bool {
	return s.settings.GetDefault(ctx, settings.KeyOpenSubtitlesKey, "") != ""
}

// MissingCount returns how many inventory items lack a Spanish subtitle.
func (s *Service) MissingCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM media_items WHERE has_subs_es = 0`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count missing subs: %w", err)
	}
	return n, nil
}

// MissingItems lists inventory items lacking a Spanish subtitle.
func (s *Service) MissingItems(ctx context.Context, limit int) ([]MissingItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, type, title, year FROM media_items WHERE has_subs_es = 0 ORDER BY type, title LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list missing subs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []MissingItem
	for rows.Next() {
		var it MissingItem
		if err := rows.Scan(&it.Path, &it.Type, &it.Title, &it.Year); err != nil {
			return nil, fmt.Errorf("scan missing item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Service) client(ctx context.Context) (*opensubtitles.Client, error) {
	key := s.settings.GetDefault(ctx, settings.KeyOpenSubtitlesKey, "")
	if key == "" {
		return nil, ErrNotConfigured
	}
	return opensubtitles.New(key), nil
}

// Search queries OpenSubtitles for Spanish subtitles matching title/year.
func (s *Service) Search(ctx context.Context, title string, year int) ([]opensubtitles.Subtitle, error) {
	c, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	return c.Search(ctx, title, year, language)
}

// Download fetches the subtitle for fileID and writes it into folder as a
// ".es.srt" file, then marks the media item as having a Spanish subtitle.
func (s *Service) Download(ctx context.Context, fileID int, folder string) (string, error) {
	c, err := s.client(ctx)
	if err != nil {
		return "", err
	}
	// Download requires a user token; log in when credentials are configured.
	user := s.settings.GetDefault(ctx, settings.KeyOpenSubtitlesUser, "")
	pass := s.settings.GetDefault(ctx, settings.KeyOpenSubtitlesPass, "")
	if user != "" && pass != "" {
		if err := c.Login(ctx, user, pass); err != nil {
			return "", err
		}
	}

	content, _, err := c.Download(ctx, fileID)
	if err != nil {
		return "", err
	}

	if info, err := os.Stat(folder); err != nil || !info.IsDir() {
		return "", fmt.Errorf("media folder not accessible: %s", folder)
	}
	dest := filepath.Join(folder, filepath.Base(folder)+".es.srt")
	if err := os.WriteFile(dest, content, 0o644); err != nil {
		return "", fmt.Errorf("write subtitle: %w", err)
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE media_items SET has_subs_es = 1 WHERE path = ?`, folder); err != nil {
		return "", fmt.Errorf("mark subtitle present: %w", err)
	}
	return dest, nil
}
