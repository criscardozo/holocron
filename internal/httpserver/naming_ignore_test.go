package httpserver

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cristian/holocron/internal/folders"
)

// TestTheIgnoreButtonWorksEndToEnd drives the whole thing over HTTP: the row
// offers the button, pressing it removes the row, and the folder shows up in
// the reversible list.
func TestTheIgnoreButtonWorksEndToEnd(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	movies := t.TempDir()
	for _, n := range []string{"Trabajo en progreso", ".claude"} {
		if err := os.MkdirAll(filepath.Join(movies, n), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ts.deps.Folders.Add(t.Context(), "Películas", movies, folders.PurposeMovies); err != nil {
		t.Fatal(err)
	}
	ts.post(t, "/naming/scan", url.Values{}, nil)

	body := ts.get(t, "/naming", nil).Body
	if !strings.Contains(body, "Trabajo en progreso") {
		t.Fatal("the badly named folder is not listed")
	}
	// The dot folder the user complained about never appears in the first place.
	if strings.Contains(body, ".claude") {
		t.Error(".claude is being flagged; hidden folders should be skipped")
	}
	if !strings.Contains(body, "/naming/ignore") {
		t.Error("no ignore button on the row")
	}

	target := filepath.Join(movies, "Trabajo en progreso")
	ts.post(t, "/naming/ignore", url.Values{"path": {target}}, nil)

	body = ts.get(t, "/naming", nil).Body
	if strings.Contains(body, "Nombre actual") && strings.Contains(body, ">Trabajo en progreso<") {
		t.Error("the ignored folder is still in the issues table")
	}
	if !strings.Contains(body, "Ignoradas") || !strings.Contains(body, "Dejar de ignorar") {
		t.Error("the ignored folder should be listed with a way back")
	}

	ts.post(t, "/naming/unignore", url.Values{"path": {target}}, nil)
	if body := ts.get(t, "/naming", nil).Body; !strings.Contains(body, "Trabajo en progreso") {
		t.Error("un-ignoring did not bring the folder back")
	}
}
