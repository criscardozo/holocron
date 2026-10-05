package arr

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// The fixtures follow QueueResource from each project's openapi.json. They are
// synthetic: when the capture was taken nothing was downloading on Ginebra, so
// there was no real queue to record. Replace them with a real capture when
// there is one.
func serve(t *testing.T, fixture string) *httptest.Server {
	t.Helper()
	body, err := os.ReadFile("testdata/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("apikey") != "" {
			t.Error("the key went in the query string, where every access log keeps it")
		}
		_, _ = w.Write(body)
	}))
}

func TestRadarrQueueNamesTheFilm(t *testing.T) {
	t.Parallel()
	srv := serve(t, "radarr_queue.json")
	defer srv.Close()
	items, err := New(Radarr, srv.URL, "k").Queue(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v", items)
	}
	it := items[0]
	if it.Subject != "Dune (2021)" {
		t.Errorf("subject = %q", it.Subject)
	}
	if it.DownloadID != "AB12CD34EF56AB12CD34EF56AB12CD34EF56AB12" {
		t.Errorf("download id = %q, want it upper-cased to match qBittorrent's hash", it.DownloadID)
	}
	if p := it.Progress(); p != 0.75 {
		t.Errorf("progress = %v", p)
	}
}

func TestSonarrQueueNamesTheEpisodeAndSaysWhyItIsStuck(t *testing.T) {
	t.Parallel()
	srv := serve(t, "sonarr_queue.json")
	defer srv.Close()
	items, err := New(Sonarr, srv.URL, "k").Queue(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d", len(items))
	}
	if got := items[0].Subject; got != "The Bear · T2E04 — Honeydew" {
		t.Errorf("subject = %q", got)
	}
	if items[0].State != "importBlocked" || len(items[0].Messages) != 1 {
		t.Errorf("a blocked import must carry its reason: %+v", items[0])
	}
	// An item with no movie or series still shows something.
	if items[1].Subject != "Something.Unmatched" {
		t.Errorf("fallback subject = %q", items[1].Subject)
	}
}

func TestARefusedKeyIsReportedAsSuch(t *testing.T) {
	t.Parallel()
	srv := serve(t, "radarr_queue.json")
	defer srv.Close()
	if _, err := New(Radarr, srv.URL, "wrong").Queue(t.Context()); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
}
