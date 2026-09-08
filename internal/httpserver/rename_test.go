package httpserver

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/folders"
)

// waitForRename polls until neither rename job is running. The work happens in
// a goroutine, so asserting straight after the POST would be a race.
func waitForRename(t *testing.T, ts *testServer) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !ts.deps.Naming.Previewing() && !ts.deps.Naming.Renaming() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the rename job never finished")
}

// setupLibrary configures a movie folder with one badly named film in it.
func setupLibrary(t *testing.T, ts *testServer) string {
	t.Helper()
	movies := t.TempDir()
	dir := filepath.Join(movies, "The.Matrix.1999.1080p.BluRay")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{
		"The.Matrix.1999.1080p.BluRay.mkv",
		"The.Matrix.1999.1080p.BluRay.es.srt",
	} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ts.deps.Folders.Add(t.Context(), "Películas", movies, folders.PurposeMovies); err != nil {
		t.Fatal(err)
	}
	return movies
}

// TestRenameNeedsAPreviewFirst. The screen must not offer to rename anything it
// has not shown, because the preview is the only thing standing between a bulk
// rename and a library nobody can put back.
func TestRenameNeedsAPreviewFirst(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	setupLibrary(t, ts)

	body := ts.get(t, "/naming/rename", nil).Body
	if strings.Contains(body, "Renombrar lo tildado") {
		t.Error("the apply button is offered before anything has been previewed")
	}
	if !strings.Contains(body, "Revisar qué cambiaría") {
		t.Error("the page should invite a preview")
	}
}

// TestApplyingNothingRenamesNothing covers the empty submit, which is what an
// impatient click on a page with everything unticked produces.
func TestApplyingNothingRenamesNothing(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	movies := setupLibrary(t, ts)

	resp := ts.post(t, "/naming/rename/apply", url.Values{}, nil)
	if !strings.Contains(resp.Body, "No tildaste ninguna carpeta") {
		t.Errorf("expected the empty-selection notice, got %q", resp.Body)
	}
	if _, err := os.Stat(filepath.Join(movies, "The.Matrix.1999.1080p.BluRay")); err != nil {
		t.Errorf("the folder was renamed anyway: %v", err)
	}
}

// TestPreviewThenApply is the whole flow through HTTP: preview, see the exact
// diff, apply only what was ticked.
func TestPreviewThenApply(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	movies := setupLibrary(t, ts)

	ts.post(t, "/naming/rename/preview", url.Values{}, nil)
	waitForRename(t, ts)

	body := ts.get(t, "/naming/rename", nil).Body
	for _, want := range []string{
		"The.Matrix.1999.1080p.BluRay",
		"The Matrix (1999)",
		"The Matrix (1999).es.srt", // the subtitle is shown moving too
		"Renombrar lo tildado",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the preview does not mention %q", want)
		}
	}

	key := filepath.Join(movies, "The.Matrix.1999.1080p.BluRay")
	ts.post(t, "/naming/rename/apply", url.Values{"folder": {key}}, nil)
	waitForRename(t, ts)

	for _, want := range []string{
		"The Matrix (1999)/The Matrix (1999).mkv",
		"The Matrix (1999)/The Matrix (1999).es.srt",
	} {
		if _, err := os.Stat(filepath.Join(movies, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
}

// TestApplyIgnoresAPathOutsideTheLibrary drives the confinement check through
// the form, since that is where the value actually comes from.
func TestApplyIgnoresAPathOutsideTheLibrary(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	setupLibrary(t, ts)

	outside := t.TempDir()
	victim := filepath.Join(outside, "No.Tocar.2001")
	if err := os.MkdirAll(victim, 0o750); err != nil {
		t.Fatal(err)
	}

	ts.post(t, "/naming/rename/apply", url.Values{"folder": {victim}}, nil)
	waitForRename(t, ts)

	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("a folder outside every media folder was renamed: %v", err)
	}
}
