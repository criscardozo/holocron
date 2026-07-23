// Package folders is the store for user-configured watched folders: the
// directories shown by the disk widget and checked by the naming validator.
package folders

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	db *sql.DB
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

// Add inserts a watched folder. The path is normalised to an absolute, cleaned
// form. Purpose defaults to disk when empty.
func (s *Store) Add(ctx context.Context, label, path, purpose string) (int64, error) {
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
	res, err := s.db.ExecContext(ctx, `DELETE FROM watched_folders WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete folder %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
