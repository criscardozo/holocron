package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"testing"

	_ "modernc.org/sqlite"
)

// TestUpgradeFromJellyfinSchema builds a database at the schema a Pi is
// upgrading from — migrations 0001 to 0003 applied, with an inventory row that
// carries an .nfo timestamp — and checks that opening it keeps the row while
// dropping the column Holocron no longer fills.
//
// The upgrade is what a real install goes through, so it is worth a test:
// getting it wrong loses an inventory that took a sync to build.
func TestUpgradeFromJellyfinSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "pre.db")

	seed(t, ctx, path,
		`CREATE TABLE schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now')))`,
		`INSERT INTO schema_migrations (version) VALUES
			('0001_init.sql'), ('0002_media_alt_ids.sql'), ('0003_jellyfin.sql')`,
		`CREATE TABLE media_items (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			path           TEXT NOT NULL UNIQUE,
			type           TEXT NOT NULL,
			title          TEXT,
			year           INTEGER,
			server_item_id TEXT,
			has_subs_es    INTEGER NOT NULL DEFAULT 0,
			nfo_written_at TEXT,
			provider_ids   TEXT NOT NULL DEFAULT '{}')`,
		`INSERT INTO media_items
			(path, type, title, year, server_item_id, has_subs_es, nfo_written_at, provider_ids)
		 VALUES
			('/media/peliculas/Dune (2021)', 'movie', 'Dune', 2021, 'abc', 1,
			 '2026-08-01 10:00:00', '{"Imdb":"tt1160419"}')`,
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`INSERT INTO settings (key, value) VALUES ('jellyfin.url', 'http://obiwan:8096')`,
	)

	database, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open (which migrates): %v", err)
	}
	defer func() { _ = database.Close() }()

	cols := column(t, database, `SELECT name FROM pragma_table_info('media_items')`)
	if slices.Contains(cols, "nfo_written_at") {
		t.Error("nfo_written_at should be gone: Holocron no longer writes .nfo files")
	}
	for _, want := range []string{"path", "title", "has_subs_es", "server_item_id", "provider_ids"} {
		if !slices.Contains(cols, want) {
			t.Errorf("column %q was lost, got %v", want, cols)
		}
	}

	// The inventory itself must survive: it costs a full sync to rebuild.
	var title string
	var subs int
	if err := database.QueryRowContext(ctx,
		`SELECT title, has_subs_es FROM media_items WHERE path = ?`,
		"/media/peliculas/Dune (2021)").Scan(&title, &subs); err != nil {
		t.Fatalf("the inventory row did not survive: %v", err)
	}
	if title != "Dune" || subs != 1 {
		t.Errorf("row = %q/%d, want Dune/1", title, subs)
	}

	if got := column(t, database, `SELECT key FROM settings`); !slices.Contains(got, "jellyfin.url") {
		t.Errorf("settings were touched: %v", got)
	}

	// The quality report table is created empty, and holds one row at most.
	if _, err := database.ExecContext(ctx,
		`INSERT INTO quality_reports (id, report) VALUES (1, '{}')`); err != nil {
		t.Fatalf("insert report: %v", err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO quality_reports (id, report) VALUES (2, '{}')`); err == nil {
		t.Error("a second report row was accepted; the CHECK should keep it a single row")
	}
}

func seed(t *testing.T, ctx context.Context, path string, stmts ...string) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	defer func() { _ = raw.Close() }()
	for _, stmt := range stmts {
		if _, err := raw.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
}

func column(t *testing.T, database *sql.DB, q string) []string {
	t.Helper()
	rows, err := database.Query(q)
	if err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}
