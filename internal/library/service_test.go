package library

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/db"
	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/settings"
)

// mockJellyfin serves one movie whose file lives at movieFile, with a Spanish
// subtitle track attached. Only the endpoints the sync uses are implemented.
func mockJellyfin(t *testing.T, movieFile string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/System/Info", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ServerName":"ObiWan","Version":"10.11.11"}`))
	})

	mux.HandleFunc("/Items", func(w http.ResponseWriter, r *http.Request) {
		// The sync must not scope the listing to a user: doing so hides films
		// that belong to a collection.
		if r.URL.Query().Get("userId") != "" {
			t.Errorf("the inventory query must not send userId, got %q", r.URL.RawQuery)
		}
		if got := r.Header.Get("Authorization"); !strings.Contains(got, "Token=") {
			t.Errorf("missing token in %q", got)
		}
		payload := map[string]any{
			"TotalRecordCount": 2,
			"Items": []any{
				map[string]any{
					"Id": "abc", "Name": "The Matrix", "Type": "Movie",
					"Path": movieFile, "ProductionYear": 1999,
					"ProviderIds": map[string]string{"Imdb": "tt0133093", "official website": "x"},
					"MediaSources": []any{map[string]any{
						"Path": movieFile,
						"MediaStreams": []any{
							map[string]any{"Type": "Subtitle", "Language": "spa", "IsExternal": true},
							map[string]any{"Type": "Audio", "Language": "eng"},
						},
					}},
				},
				// An episode Jellyfin knows about but has never seen on disk.
				// The sync must skip it rather than inventory a missing file.
				map[string]any{
					"Id": "ghost", "Name": "Deleted scene", "Type": "Movie", "Path": "",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(payload)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSyncFromJellyfin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dir := t.TempDir()
	movieDir := filepath.Join(dir, "The Matrix (1999)")
	if err := os.MkdirAll(movieDir, 0o755); err != nil {
		t.Fatal(err)
	}
	movieFile := filepath.Join(movieDir, "The Matrix (1999).mkv")
	if err := os.WriteFile(movieFile, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := mockJellyfin(t, movieFile)

	database, err := db.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = database.Close() }()

	store := settings.NewStore(database)
	for k, v := range map[string]string{
		settings.KeyJellyfinURL:      srv.URL,
		settings.KeyJellyfinToken:    "tok",
		settings.KeyJellyfinDeviceID: "dev",
	} {
		if err := store.Set(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}

	svc := NewService(database, store, jobs.NewManager())
	if !svc.Configured(ctx) {
		t.Fatal("expected the service to read as configured")
	}
	if info, err := svc.TestConnection(ctx); err != nil || info.Name != "ObiWan" {
		t.Fatalf("TestConnection = %+v, %v", info, err)
	}

	job, err := svc.StartSync(ctx)
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	waitForJob(t, svc, job.ID)

	stats, err := svc.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	// Only the title with a file is inventoried; the ghost is skipped.
	if stats.Total != 1 {
		t.Fatalf("total = %d, want 1 (the ghost must be skipped)", stats.Total)
	}
	// Jellyfin reported a Spanish subtitle, so nothing is missing — and no
	// directory was walked to find that out.
	if stats.WithoutSubs != 0 {
		t.Errorf("withoutSubs = %d, want 0", stats.WithoutSubs)
	}

	items, err := svc.Items(ctx, 10)
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	it := items[0]
	if it.Title != "The Matrix" || it.Year != 1999 || it.Type != "movie" {
		t.Errorf("unexpected item: %+v", it)
	}
	// The inventory records the folder, not the file.
	if it.Path != movieDir {
		t.Errorf("path = %q, want the folder %q", it.Path, movieDir)
	}
	if !it.HasSubsES {
		t.Error("expected the Spanish subtitle to be recorded")
	}
}

func waitForJob(t *testing.T, svc *Service, id string) {
	t.Helper()
	for range 200 {
		if job, ok := svc.jobs.Get(id); ok && job.Status != jobs.StatusRunning {
			if job.Status == jobs.StatusError {
				t.Fatalf("job failed: %s", job.Err)
			}
			return
		}
		waitTick()
	}
	t.Fatal("job did not finish")
}

func waitTick() { time.Sleep(10 * time.Millisecond) }
