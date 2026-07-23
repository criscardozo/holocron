// Package db opens the SQLite database and applies embedded migrations. It uses
// the pure-Go modernc.org/sqlite driver so the binary cross-compiles to ARM
// without CGO.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Open opens (creating if needed) the SQLite database at path, applies pending
// migrations, and returns the connection. The parent directory is created and
// the file is restricted to owner-only access because it holds secrets.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	dsn := "file:" + path + "?" + url.Values{
		"_pragma": {"busy_timeout(5000)", "journal_mode(WAL)", "foreign_keys(on)"},
	}.Encode()

	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// modernc.org/sqlite serialises writes; a single connection avoids
	// "database is locked" churn on a low-powered device.
	database.SetMaxOpenConns(1)

	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	// Restrict the file now that it exists.
	_ = os.Chmod(path, 0o600)

	if err := migrate(ctx, database); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return database, nil
}
