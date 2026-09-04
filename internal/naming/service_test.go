package naming

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cristian/holocron/internal/db"
	"github.com/cristian/holocron/internal/folders"
)

func newService(t *testing.T) (*Service, *folders.Store) {
	t.Helper()
	database, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	store := folders.NewStore(database)
	return NewService(database, store), store
}

func mkdirs(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.MkdirAll(filepath.Join(root, name), 0o750); err != nil {
			t.Fatal(err)
		}
	}
}

func TestScanRecordsIssuesAndReplacesThemEachRun(t *testing.T) {
	t.Parallel()
	svc, store := newService(t)

	movies := t.TempDir()
	shows := t.TempDir()
	mkdirs(t, movies, "The Matrix", "Dune (2021)")
	mkdirs(t, shows, "the.bear.s01")

	for _, f := range []struct{ label, path, purpose string }{
		{"Películas", movies, folders.PurposeMovies},
		{"Series", shows, folders.PurposeTV},
		// A disk folder is not a media folder and must not be scanned, or every
		// directory on the drive would be reported.
		{"Disco", t.TempDir(), folders.PurposeDisk},
	} {
		if _, err := store.Add(t.Context(), f.label, f.path, f.purpose); err != nil {
			t.Fatal(err)
		}
	}
	mkdirs(t, t.TempDir(), "whatever")

	n, err := svc.Scan(t.Context())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if n != 2 {
		t.Fatalf("found %d issues, want 2", n)
	}

	issues, err := svc.Issues(t.Context())
	if err != nil {
		t.Fatalf("Issues: %v", err)
	}
	byFound := map[string]Issue{}
	for _, is := range issues {
		byFound[is.Found] = is
	}
	if got, ok := byFound["The Matrix"]; !ok || got.Type != "movies" {
		t.Errorf("expected the film reported as movies, got %+v", got)
	}
	if got, ok := byFound["the.bear.s01"]; !ok || got.Type != "tv" {
		t.Errorf("expected the show reported as tv, got %+v", got)
	}

	// Fixing a folder and rescanning must drop the old row: the count feeds an
	// attention chip on the dashboard, and a stale issue keeps it lit forever.
	if err := os.Rename(filepath.Join(movies, "The Matrix"),
		filepath.Join(movies, "The Matrix (1999)")); err != nil {
		t.Fatal(err)
	}
	if n, err = svc.Scan(t.Context()); err != nil || n != 1 {
		t.Fatalf("rescan = %d, %v; want 1", n, err)
	}
	if count, err := svc.Count(t.Context()); err != nil || count != 1 {
		t.Errorf("Count = %d, %v; want 1", count, err)
	}
}

// TestScanSurvivesAnUnavailableFolder: a watched folder can be on a disk that
// is unmounted. Skipping it keeps the other folders reportable, which beats
// failing the whole scan.
func TestScanSurvivesAnUnavailableFolder(t *testing.T) {
	t.Parallel()
	svc, store := newService(t)

	good := t.TempDir()
	mkdirs(t, good, "The Matrix")
	if _, err := store.Add(t.Context(), "Películas", good, folders.PurposeMovies); err != nil {
		t.Fatal(err)
	}
	// Added while it exists, then taken away: the store rejects a path that is
	// not there, so an unmounted disk can only be simulated this way — which is
	// also exactly how it happens in real life.
	external := t.TempDir()
	if _, err := store.Add(t.Context(), "Externo", external, folders.PurposeMovies); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(external); err != nil {
		t.Fatal(err)
	}

	n, err := svc.Scan(t.Context())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if n != 1 {
		t.Errorf("found %d issues, want 1 from the folder that is there", n)
	}
}

func TestHasMediaFoldersDrivesTheEmptyState(t *testing.T) {
	t.Parallel()
	svc, store := newService(t)

	if svc.HasMediaFolders(t.Context()) {
		t.Error("nothing is configured yet")
	}
	// A disk folder is not a media folder: the naming screen would otherwise
	// say "no issues" when it has nothing to look at.
	if _, err := store.Add(t.Context(), "Disco", t.TempDir(), folders.PurposeDisk); err != nil {
		t.Fatal(err)
	}
	if svc.HasMediaFolders(t.Context()) {
		t.Error("a disk folder is not a media folder")
	}
	if _, err := store.Add(t.Context(), "Series", t.TempDir(), folders.PurposeTV); err != nil {
		t.Fatal(err)
	}
	if !svc.HasMediaFolders(t.Context()) {
		t.Error("a tv folder counts")
	}
}
