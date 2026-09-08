package naming

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cristian/holocron/internal/folders"
)

// TestLocateRefusesAnythingOutsideAMediaFolder. Keys come back from a form, so
// they are user input on the way to a rename. This is the check that decides
// whether that rename can reach the rest of the disk.
func TestLocateRefusesAnythingOutsideAMediaFolder(t *testing.T) {
	t.Parallel()
	roots := []folders.Folder{
		{Path: "/mnt/grande/Peliculas", Label: "Películas"},
		{Path: "/mnt/grande/Series", Label: "Series"},
	}

	for _, key := range []string{
		"/etc/systemd/system",
		"/mnt/grande",
		"/mnt/grande/Peliculas",                  // the media folder itself
		"/mnt/grande/Peliculas-viejas/Heat.1995", // a prefix, not a child
		"/mnt/grande/Peliculas/Heat.1995/Subs",   // deeper than a film
		"/mnt/grande/Peliculas/../../etc/passwd",
		"/mnt/grande/Peliculas/Heat.1995/../../../tmp",
		"",
	} {
		if root, rel, ok := locate(roots, key); ok {
			t.Errorf("locate(%q) allowed root=%q rel=%q", key, root, rel)
		}
	}
}

// TestLocateAcceptsARealFilm, so the guard above is not simply refusing
// everything — a check that never says yes passes every negative test.
func TestLocateAcceptsARealFilm(t *testing.T) {
	t.Parallel()
	roots := []folders.Folder{{Path: "/mnt/grande/Peliculas"}}
	root, rel, ok := locate(roots, "/mnt/grande/Peliculas/Heat.1995")
	if !ok || root != "/mnt/grande/Peliculas" || rel != "Heat.1995" {
		t.Fatalf("locate = %q, %q, %v", root, rel, ok)
	}
	if _, rel, ok := locate(roots, "/mnt/grande/Peliculas/Heat.1995/"); !ok || rel != "Heat.1995" {
		t.Errorf("a trailing slash broke it: rel=%q ok=%v", rel, ok)
	}
}

// TestApplyStaysInsideTheMediaFolder drives the same guard through the public
// entry point against real directories, so confinement is tested where it is
// used and not only in the helper.
func TestApplyStaysInsideTheMediaFolder(t *testing.T) {
	t.Parallel()
	svc, store := newService(t)

	base := t.TempDir()
	movies := filepath.Join(base, "Peliculas")
	private := filepath.Join(base, "Privado")
	mkdirs(t, movies, "Heat.1995")
	mkdirs(t, private, "No.Tocar.2001")

	if _, err := store.Add(t.Context(), "Películas", movies, folders.PurposeMovies); err != nil {
		t.Fatal(err)
	}

	sum, err := svc.ApplyFolders(t.Context(), []string{
		filepath.Join(private, "No.Tocar.2001"),
		filepath.Join(movies, "..", "Privado", "No.Tocar.2001"),
		filepath.Join(movies, "Heat.1995", ".."), // resolves to the media folder
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Folders != 0 || sum.Files != 0 {
		t.Fatalf("something outside was renamed: %+v", sum)
	}
	if len(sum.Failed) != 3 {
		t.Errorf("every refusal must be reported, got %+v", sum.Failed)
	}
	if _, err := os.Stat(filepath.Join(private, "No.Tocar.2001")); err != nil {
		t.Errorf("the folder outside was touched: %v", err)
	}
}

// TestPlanThenApplyRenamesTheWholeFolder is the happy path end to end, through
// the service rather than the planner, because that is where the media folder
// lookup and the os.Root confinement actually get wired together.
func TestPlanThenApplyRenamesTheWholeFolder(t *testing.T) {
	t.Parallel()
	svc, store := newService(t)

	movies := t.TempDir()
	mkdirs(t, movies, "The.Matrix.1999.1080p.BluRay", "Dune (2021)")
	write(t, movies, "The.Matrix.1999.1080p.BluRay/The.Matrix.1999.1080p.BluRay.mkv")
	write(t, movies, "The.Matrix.1999.1080p.BluRay/The.Matrix.1999.1080p.BluRay.es.srt")
	write(t, movies, "Dune (2021)/Dune (2021).mkv")

	if _, err := store.Add(t.Context(), "Películas", movies, folders.PurposeMovies); err != nil {
		t.Fatal(err)
	}

	plans, err := svc.PlanMovies(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// The already-correct folder must not appear: a preview listing work that
	// is not needed trains people to apply without reading.
	if len(plans) != 1 || plans[0].Plan.Folder != "The.Matrix.1999.1080p.BluRay" {
		t.Fatalf("plans = %+v", plans)
	}
	if plans[0].Plan.NewFolder != "The Matrix (1999)" {
		t.Fatalf("NewFolder = %q", plans[0].Plan.NewFolder)
	}

	sum, err := svc.ApplyFolders(t.Context(), []string{plans[0].Key()})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Folders != 1 || sum.Files != 2 {
		t.Fatalf("summary = %+v", sum)
	}

	for _, want := range []string{
		"The Matrix (1999)/The Matrix (1999).mkv",
		"The Matrix (1999)/The Matrix (1999).es.srt",
		"Dune (2021)/Dune (2021).mkv",
	} {
		if _, err := os.Stat(filepath.Join(movies, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}

	// And a second preview must find nothing left to do.
	again, err := svc.PlanMovies(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Errorf("a second pass still wants to change things: %+v", again)
	}
}

func write(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, rel), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}
