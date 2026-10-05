package naming

import (
	"path/filepath"
	"testing"

	"github.com/cristian/holocron/internal/folders"
)

// TestHiddenFoldersAreNeverFlagged. A tool's own directory next to the films,
// and the bookkeeping exFAT and macOS leave behind, will never satisfy
// "Título (Año)". Flagging them forever teaches people to skim the list, which
// is how a real problem gets missed.
func TestHiddenFoldersAreNeverFlagged(t *testing.T) {
	t.Parallel()
	svc, store := newService(t)

	movies := t.TempDir()
	mkdirs(t, movies, ".claude", ".Spotlight-V100", ".Trashes",
		"System Volume Information", "The Matrix")
	if _, err := store.Add(t.Context(), "Películas", movies, folders.PurposeMovies); err != nil {
		t.Fatal(err)
	}

	n, err := svc.Scan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		issues, _ := svc.Issues(t.Context())
		t.Fatalf("flagged %d folders, want only the real one: %+v", n, issues)
	}
	issues, err := svc.Issues(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if issues[0].Found != "The Matrix" {
		t.Errorf("flagged %q", issues[0].Found)
	}
}

// TestIgnoringAFolderTakesItOffEveryList — the issue list and the count. A folder that still shows up somewhere after being ignored
// is worse than no button at all.
func TestIgnoringAFolderTakesItOffEveryList(t *testing.T) {
	t.Parallel()
	svc, store := newService(t)

	movies := t.TempDir()
	mkdirs(t, movies, "Trabajo en progreso", "The.Matrix.1999")
	if _, err := store.Add(t.Context(), "Películas", movies, folders.PurposeMovies); err != nil {
		t.Fatal(err)
	}

	if n, err := svc.Scan(t.Context()); err != nil || n != 2 {
		t.Fatalf("scan = %d, %v; want 2", n, err)
	}

	target := filepath.Join(movies, "Trabajo en progreso")
	if err := svc.Ignore(t.Context(), target); err != nil {
		t.Fatal(err)
	}

	n, err := svc.Scan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("scan still counts %d, want 1", n)
	}
	if c, _ := svc.Count(t.Context()); c != 1 {
		t.Errorf("Count = %d, want 1", c)
	}
}

// TestIgnoringIsReversible, because a one-way button quietly shrinks the list
// until nobody remembers what is missing from it.
func TestIgnoringIsReversible(t *testing.T) {
	t.Parallel()
	svc, store := newService(t)

	movies := t.TempDir()
	mkdirs(t, movies, "Trabajo en progreso")
	if _, err := store.Add(t.Context(), "Películas", movies, folders.PurposeMovies); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(movies, "Trabajo en progreso")

	if err := svc.Ignore(t.Context(), target); err != nil {
		t.Fatal(err)
	}
	// Twice is not an error: two tabs, or an impatient double click.
	if err := svc.Ignore(t.Context(), target); err != nil {
		t.Fatalf("ignoring twice failed: %v", err)
	}
	paths, err := svc.IgnoredPaths(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != target {
		t.Fatalf("IgnoredPaths = %v", paths)
	}

	if err := svc.Unignore(t.Context(), target); err != nil {
		t.Fatal(err)
	}
	if n, _ := svc.Scan(t.Context()); n != 1 {
		t.Errorf("after un-ignoring, scan = %d, want the folder back", n)
	}
}
