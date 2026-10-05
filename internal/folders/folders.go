// Package folders is the store for user-configured watched folders: the
// directories shown by the disk widget and checked by the naming validator.
package folders

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Purpose classifies what a folder is watched for.
const (
	PurposeDisk   = "disk"
	PurposeMovies = "movies"
	PurposeTV     = "tv"
)

// Folder is a watched directory.
type Folder struct {
	ID        int64
	Label     string
	Path      string
	Purpose   string
	CreatedAt string
}

// ErrNotFound is returned when a folder id does not exist.
var ErrNotFound = errors.New("folder not found")

// Store provides CRUD access to watched folders.
type Store struct {
	db      *sql.DB
	managed bool
}

// NewStore creates a Store.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// List returns watched folders, optionally filtered by purpose ("" = all).
func (s *Store) List(ctx context.Context, purpose string) ([]Folder, error) {
	query := `SELECT id, label, path, purpose, created_at FROM watched_folders`
	args := []any{}
	if purpose != "" {
		query += ` WHERE purpose = ?`
		args = append(args, purpose)
	}
	query += ` ORDER BY label`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Folder
	for rows.Next() {
		var f Folder
		if err := rows.Scan(&f.ID, &f.Label, &f.Path, &f.Purpose, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan folder: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// Get returns the folder with the given id.
func (s *Store) Get(ctx context.Context, id int64) (Folder, error) {
	var f Folder
	err := s.db.QueryRowContext(ctx,
		`SELECT id, label, path, purpose, created_at FROM watched_folders WHERE id = ?`, id).
		Scan(&f.ID, &f.Label, &f.Path, &f.Purpose, &f.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Folder{}, ErrNotFound
	}
	if err != nil {
		return Folder{}, fmt.Errorf("get folder %d: %w", id, err)
	}
	return f, nil
}

// ErrNotADirectory is returned when the path to watch does not exist or is not
// a directory.
var ErrNotADirectory = errors.New("path is not an existing directory")

// Add inserts a watched folder. The path is normalised to an absolute, cleaned
// form and must already exist as a directory, so a typo surfaces here instead
// of as an unexplained "no se pudo leer" later. Purpose defaults to disk.
func (s *Store) Add(ctx context.Context, label, path, purpose string) (int64, error) {
	if s.managed {
		return 0, ErrManaged
	}
	if label == "" || path == "" {
		return 0, errors.New("label and path are required")
	}
	if purpose == "" {
		purpose = PurposeDisk
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, fmt.Errorf("normalise path: %w", err)
	}
	abs = filepath.Clean(abs)

	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return 0, fmt.Errorf("%w: %s", ErrNotADirectory, abs)
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO watched_folders (label, path, purpose) VALUES (?, ?, ?)`,
		label, abs, purpose)
	if err != nil {
		return 0, fmt.Errorf("insert folder: %w", err)
	}
	return res.LastInsertId()
}

// Delete removes a watched folder (and, via cascade, its cached scan).
func (s *Store) Delete(ctx context.Context, id int64) error {
	if s.managed {
		return ErrManaged
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM watched_folders WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete folder %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ErrManaged means the folders come from the server's configuration and are
// not edited from the UI.
var ErrManaged = errors.New("folders are managed by the server")

// Spec is one folder as the server configures it.
type Spec struct {
	Label, Purpose, Path string
}

// Manage makes the watched folders exactly specs, and stops the UI editing
// them. Same reasoning as the credentials systemd provides: on Ginebra the
// server knows what its library is, and a form that disagrees with it is a
// second source of truth.
//
// Reconciled in the database rather than kept beside it, because disk scans
// are cached against a folder's id: a folder that keeps its path keeps its id
// and its last scan. The paths are not checked to exist. Holocron has to start
// when a USB disk has fallen off the bus — that is exactly when its screens are
// most needed — and the disk screen says the folder is unavailable.
func (s *Store) Manage(ctx context.Context, specs []Spec) error {
	if len(specs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	keep := map[string]bool{}
	for _, sp := range specs {
		p := filepath.Clean(sp.Path)
		if !filepath.IsAbs(p) || sp.Label == "" {
			return fmt.Errorf("managed folder %q: needs a label and an absolute path", sp.Path)
		}
		purpose := sp.Purpose
		switch purpose {
		case PurposeDisk, PurposeMovies, PurposeTV:
		default:
			return fmt.Errorf("managed folder %q: purpose %q is not disk, movies or tv", sp.Path, purpose)
		}
		keep[p] = true
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO watched_folders (label, path, purpose) VALUES (?, ?, ?)
			 ON CONFLICT(path) DO UPDATE SET label = excluded.label, purpose = excluded.purpose`,
			sp.Label, p, purpose); err != nil {
			return fmt.Errorf("upsert folder %q: %w", p, err)
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, path FROM watched_folders`)
	if err != nil {
		return fmt.Errorf("list folders: %w", err)
	}
	var drop []int64
	for rows.Next() {
		var id int64
		var p string
		if err := rows.Scan(&id, &p); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan folder: %w", err)
		}
		if !keep[p] {
			drop = append(drop, id)
		}
	}
	_ = rows.Close()
	for _, id := range drop {
		if _, err := tx.ExecContext(ctx, `DELETE FROM watched_folders WHERE id = ?`, id); err != nil {
			return fmt.Errorf("drop folder %d: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.managed = true
	return nil
}

// Managed reports whether the server owns the folder list.
func (s *Store) Managed() bool { return s.managed }
