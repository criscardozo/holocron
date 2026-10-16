package naming

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

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

	// A folder that cannot be read is skipped, not fatal: one library on a
	// sleeping or unplugged disk should not hide the others. But it is kept,
	// and shown: skipped quietly, it read as «everything complies».
	var issues []Issue
	var unreadable []string
	for _, group := range []struct {
		folders []folders.Folder
		kind    string
	}{{movies, "movies"}, {tv, "tv"}} {
		for _, f := range group.folders {
			found, err := ScanDir(f.Path, group.kind)
			if err != nil {
				unreadable = append(unreadable, f.Path)
				continue
			}
			issues = append(issues, found...)
		}
	}

	// Filtered here rather than when reading the table, so an ignored folder
	// never reaches the count either — a badge saying 3 over a list of 2 is
	// its own small bug.
	ignored, err := s.ignoredSet(ctx)
	if err != nil {
		return 0, err
	}
	kept := issues[:0]
	for _, is := range issues {
		if !ignored[is.Path] {
			kept = append(kept, is)
		}
	}
	issues = kept

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
	// When it ran, and what it could not read, beside the result: an empty
	// table means «all good» only if a scan put it there.
	for k, v := range map[string]string{
		keyScannedAt:  time.Now().UTC().Format(time.RFC3339),
		keyUnreadable: strings.Join(unreadable, "\n"),
	} {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO settings (key, value) VALUES (?, ?)
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = datetime('now')`,
			k, v); err != nil {
			return 0, fmt.Errorf("record scan: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return len(issues), nil
}

// Settings keys for the last scan. Not settings anyone edits; the table is
// simply where one-row facts live.
const (
	keyScannedAt  = "naming.scanned_at"
	keyUnreadable = "naming.unreadable"
)

// LastScan is when the folders were last checked, and which could not be
// read then. ok is false when they never were.
func (s *Service) LastScan(ctx context.Context) (at time.Time, unreadable []string, ok bool, err error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value FROM settings WHERE key IN (?, ?)`, keyScannedAt, keyUnreadable)
	if err != nil {
		return time.Time{}, nil, false, fmt.Errorf("read last scan: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return time.Time{}, nil, false, fmt.Errorf("scan last scan: %w", err)
		}
		switch k {
		case keyScannedAt:
			if t, perr := time.Parse(time.RFC3339, v); perr == nil {
				at, ok = t, true
			}
		case keyUnreadable:
			if v != "" {
				unreadable = strings.Split(v, "\n")
			}
		}
	}
	return at, unreadable, ok, rows.Err()
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
