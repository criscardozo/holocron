package naming

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/cristian/holocron/internal/folders"
)

// Service scans the configured movie/TV folders for naming issues and caches
// them in the naming_issues table. Scanning is cheap (top-level readdir plus a
// regex), so it runs synchronously.
type Service struct {
	db      *sql.DB
	folders *folders.Store
}

// NewService creates a Service.
func NewService(db *sql.DB, fs *folders.Store) *Service {
	return &Service{db: db, folders: fs}
}

// Scan re-scans all movie and TV folders, replaces the cached issues, and
// returns the number found.
func (s *Service) Scan(ctx context.Context) (int, error) {
	movies, err := s.folders.List(ctx, folders.PurposeMovies)
	if err != nil {
		return 0, fmt.Errorf("list movie folders: %w", err)
	}
	tv, err := s.folders.List(ctx, folders.PurposeTV)
	if err != nil {
		return 0, fmt.Errorf("list tv folders: %w", err)
	}

	var issues []Issue
	for _, f := range movies {
		found, err := ScanDir(f.Path, "movies")
		if err != nil {
			continue // folder may be temporarily unavailable; skip
		}
		issues = append(issues, found...)
	}
	for _, f := range tv {
		found, err := ScanDir(f.Path, "tv")
		if err != nil {
			continue
		}
		issues = append(issues, found...)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM naming_issues`); err != nil {
		return 0, fmt.Errorf("clear issues: %w", err)
	}
	for _, is := range issues {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO naming_issues (path, type, expected, found) VALUES (?, ?, ?, ?)`,
			is.Path, is.Type, is.Expected, is.Found); err != nil {
			return 0, fmt.Errorf("insert issue: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return len(issues), nil
}

// Issues returns the cached naming issues.
func (s *Service) Issues(ctx context.Context) ([]Issue, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, type, found, expected FROM naming_issues ORDER BY type, found`)
	if err != nil {
		return nil, fmt.Errorf("query issues: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Issue
	for rows.Next() {
		var is Issue
		if err := rows.Scan(&is.Path, &is.Type, &is.Found, &is.Expected); err != nil {
			return nil, fmt.Errorf("scan issue: %w", err)
		}
		out = append(out, is)
	}
	return out, rows.Err()
}

// Count returns the number of cached naming issues.
func (s *Service) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM naming_issues`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count issues: %w", err)
	}
	return n, nil
}

// HasMediaFolders reports whether any movie or TV folder is configured.
func (s *Service) HasMediaFolders(ctx context.Context) bool {
	movies, _ := s.folders.List(ctx, folders.PurposeMovies)
	if len(movies) > 0 {
		return true
	}
	tv, _ := s.folders.List(ctx, folders.PurposeTV)
	return len(tv) > 0
}
