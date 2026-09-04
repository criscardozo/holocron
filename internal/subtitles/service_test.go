package subtitles

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cristian/holocron/internal/db"
	"github.com/cristian/holocron/internal/settings"
)

func write(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSubtitleBaseName(t *testing.T) {
	t.Parallel()

	t.Run("uses the largest video file so players pair the subtitle", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		write(t, filepath.Join(dir, "Dune.Part.Two.2024.1080p.mkv"), 4096)
		write(t, filepath.Join(dir, "sample.mkv"), 16)
		write(t, filepath.Join(dir, "movie.nfo"), 32)

		if got, want := subtitleBaseName(dir), "Dune.Part.Two.2024.1080p"; got != want {
			t.Errorf("subtitleBaseName = %q, want %q", got, want)
		}
	})

	t.Run("falls back to the folder name when there is no video", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "Interstellar (2014)")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(dir, "movie.nfo"), 32)

		if got, want := subtitleBaseName(dir), "Interstellar (2014)"; got != want {
			t.Errorf("subtitleBaseName = %q, want %q", got, want)
		}
	})

	t.Run("falls back to the folder name when the folder is unreadable", func(t *testing.T) {
		t.Parallel()
		missing := filepath.Join(t.TempDir(), "does-not-exist")
		if got, want := subtitleBaseName(missing), "does-not-exist"; got != want {
			t.Errorf("subtitleBaseName = %q, want %q", got, want)
		}
	})
}

// newService wires a service over a temporary database, pointed at a fake
// OpenSubtitles.
func newService(t *testing.T, baseURL string) (*Service, *settings.Store, *sql.DB) {
	t.Helper()
	database, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	store := settings.NewStore(database)
	if err := store.Set(t.Context(), settings.KeyOpenSubtitlesKey, "key"); err != nil {
		t.Fatal(err)
	}
	svc := NewService(database, store)
	svc.baseURL = baseURL
	return svc, store, database
}

// TestDownloadRefusesAFolderNotInTheInventory is the one that matters most in
// this package. The folder comes from the client, and the write lands wherever
// it points: this check is all that stands between an API call and an arbitrary
// file write anywhere the service user can reach.
func TestDownloadRefusesAFolderNotInTheInventory(t *testing.T) {
	t.Parallel()

	var reached bool
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	defer fake.Close()

	svc, _, _ := newService(t, fake.URL)
	victim := t.TempDir()

	for _, folder := range []string{victim, "/etc", "", "../../etc"} {
		if _, err := svc.Download(t.Context(), 42, folder); err == nil {
			t.Errorf("Download into %q was allowed", folder)
		}
		if entries, _ := os.ReadDir(victim); len(entries) != 0 {
			t.Fatalf("something was written into %q", victim)
		}
	}
	if reached {
		t.Error("the refusal must happen before any request is made")
	}
}

func TestDownloadWritesBesideTheMediaAndMarksIt(t *testing.T) {
	t.Parallel()

	const srt = "1\n00:00:01,000 --> 00:00:02,000\nHola\n"
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/download"):
			// The API answers with a link, and the file is fetched from it.
			_, _ = w.Write([]byte(`{"link":"` + linkBase(r) + `/file.srt","file_name":"file.srt"}`))
		default:
			_, _ = w.Write([]byte(srt))
		}
	}))
	defer fake.Close()

	svc, _, database := newService(t, fake.URL)

	folder := t.TempDir()
	write(t, filepath.Join(folder, "Dune.Part.Two.2024.1080p.mkv"), 4096)
	if _, err := database.ExecContext(t.Context(),
		`INSERT INTO media_items (path, type, title, year, has_subs_es) VALUES (?, 'movie', 'Dune', 2024, 0)`,
		folder); err != nil {
		t.Fatal(err)
	}

	dest, err := svc.Download(t.Context(), 7, folder)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	// Named after the largest video file, so players pair the two.
	want := filepath.Join(folder, "Dune.Part.Two.2024.1080p.es.srt")
	if dest != want {
		t.Errorf("wrote %q, want %q", dest, want)
	}
	content, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read subtitle: %v", err)
	}
	if string(content) != srt {
		t.Errorf("content = %q", content)
	}

	// World-readable on purpose: Jellyfin runs as another user.
	info, err := os.Stat(want)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o044 == 0 {
		t.Errorf("mode = %v; Jellyfin runs as another user and has to read it", perm)
	}

	var marked int
	if err := database.QueryRowContext(t.Context(),
		`SELECT has_subs_es FROM media_items WHERE path = ?`, folder).Scan(&marked); err != nil {
		t.Fatal(err)
	}
	if marked != 1 {
		t.Error("the inventory should record that the subtitle is now there")
	}
}

// linkBase rebuilds the fake server's own address from the request, so the
// download link points back at it.
func linkBase(r *http.Request) string {
	return "http://" + r.Host
}
