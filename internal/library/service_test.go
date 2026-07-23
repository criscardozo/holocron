package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/db"
	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/settings"
)

// mockPlex serves the minimal Plex endpoints the sync uses. movieFile is the
// file path reported for the single movie.
func mockPlex(t *testing.T, movieFile string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/library/sections", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") == "" {
			t.Error("missing X-Plex-Token header")
		}
		_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[{"key":"1","type":"movie","title":"Movies"}]}}`))
	})
	mux.HandleFunc("/library/sections/1/all", func(w http.ResponseWriter, r *http.Request) {
		// Second page is empty, ending pagination.
		if r.Header.Get("X-Plex-Container-Start") != "0" {
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{
			"ratingKey":"10","type":"movie","title":"The Matrix","year":1999,
			"guid":"plex://movie/abc",
			"Guid":[{"id":"imdb://tt0133093"},{"id":"tmdb://603"}],
			"Media":[{"Part":[{"file":"` + movieFile + `"}]}]
		}]}}`))
	})
	return httptest.NewServer(mux)
}

func TestSyncAndGenerateNFO(t *testing.T) {
	ctx := context.Background()

	// Media folder on disk so .nfo can be written and subs detected.
	movieDir := filepath.Join(t.TempDir(), "The Matrix (1999)")
	if err := os.MkdirAll(movieDir, 0o755); err != nil {
		t.Fatal(err)
	}
	movieFile := filepath.Join(movieDir, "The Matrix (1999).mkv")
	if err := os.WriteFile(movieFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A Spanish subtitle sitting next to it.
	if err := os.WriteFile(filepath.Join(movieDir, "The Matrix.es.srt"), []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := mockPlex(t, movieFile)
	defer srv.Close()

	database, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()

	st := settings.NewStore(database)
	_ = st.Set(ctx, settings.KeyPlexURL, srv.URL)
	_ = st.Set(ctx, settings.KeyPlexToken, "token")

	svc := NewService(database, st, jobs.NewManager())

	// Sync.
	if _, err := svc.StartSync(ctx); err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	waitIdle(t, svc.Syncing)

	stats, err := svc.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 1 {
		t.Fatalf("Total = %d, want 1", stats.Total)
	}
	if stats.WithoutSubs != 0 {
		t.Errorf("WithoutSubs = %d, want 0 (Spanish sub present)", stats.WithoutSubs)
	}

	// Generate .nfo.
	if _, err := svc.StartGenerateNFO(ctx); err != nil {
		t.Fatalf("StartGenerateNFO: %v", err)
	}
	waitIdle(t, svc.GeneratingNFO)

	nfoPath := filepath.Join(movieDir, "movie.nfo")
	data, err := os.ReadFile(nfoPath)
	if err != nil {
		t.Fatalf("expected movie.nfo written: %v", err)
	}
	for _, want := range []string{"The Matrix", "1999", "plex://movie/abc", "tt0133093", `language="spa">yes`} {
		if !contains(string(data), want) {
			t.Errorf("movie.nfo missing %q; got:\n%s", want, data)
		}
	}
}

func waitIdle(t *testing.T, running func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !running() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job did not finish in time")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
