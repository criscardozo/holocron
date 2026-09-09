package naming

import (
	"os"
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

// TestIgnoringAFolderTakesItOffEveryList — the issue list, the count, and the
// rename preview. A folder that still shows up somewhere after being ignored
// is worse than no button at all.
func TestIgnoringAFolderTakesItOffEveryList(t *testing.T) {
	t.Parallel()
	svc, store := newService(t)

	movies := t.TempDir()
	mkdirs(t, movies, "Trabajo en progreso", "The.Matrix.1999")
	write(t, movies, "The.Matrix.1999/The.Matrix.1999.mkv")
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
	plans, err := svc.PlanMovies(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range plans {
		if p.Plan.Folder == "Trabajo en progreso" {
			t.Error("the rename preview still offers to touch an ignored folder")
		}
	}
}

// TestAnIgnoredFolderIsNotRenamedEvenIfAsked. The preview and the apply are
// separate requests, so an ignore added in between has to win — that is the
// one case the button exists for.
func TestAnIgnoredFolderIsNotRenamedEvenIfAsked(t *testing.T) {
	t.Parallel()
	svc, store := newService(t)

	movies := t.TempDir()
	mkdirs(t, movies, "The.Matrix.1999")
	write(t, movies, "The.Matrix.1999/The.Matrix.1999.mkv")
	if _, err := store.Add(t.Context(), "Películas", movies, folders.PurposeMovies); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(movies, "The.Matrix.1999")
	if err := svc.Ignore(t.Context(), target); err != nil {
		t.Fatal(err)
	}
	sum, err := svc.ApplyFolders(t.Context(), []string{target})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Folders != 0 || sum.Files != 0 {
		t.Fatalf("renamed an ignored folder: %+v", sum)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("the folder was touched: %v", err)
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
