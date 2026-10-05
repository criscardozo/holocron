package activity

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/arr"
	"github.com/cristian/holocron/internal/bazarr"
)

// fakeProwlarr serves the real indexer list captured on Ginebra (only id,
// name, enable, protocol and privacy kept: the rest of each record is the
// indexer's own configuration), with one indexer in backoff.
func fakeProwlarr(t *testing.T, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	idx, err := os.ReadFile("testdata/prowlarr_indexers.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/api/v1/indexer":
			_, _ = w.Write(idx)
		case "/api/v1/indexerstatus":
			future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
			past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
			// 1337x is backing off; another indexer recovered an hour ago.
			_, _ = w.Write([]byte(`[{"indexerId":1,"disabledTill":"` + future + `"},{"indexerId":2,"disabledTill":"` + past + `"}]`))
		case "/api/v1/health":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestOnlyIndexersStillBackingOffAreFailing(t *testing.T) {
	t.Parallel()
	var hits atomic.Int64
	srv := fakeProwlarr(t, &hits)
	l := NewSlow(SlowSources{Prowlarr: arr.New(arr.Prowlarr, srv.URL, "k")}).Read(t.Context(), time.Now())
	if l.Indexers != 5 || l.IndexersEnabled != 5 {
		t.Errorf("indexers = %d (%d enabled)", l.Indexers, l.IndexersEnabled)
	}
	if len(l.Failing) != 1 || l.Failing[0] != "1337x" {
		t.Errorf("failing = %v, want only the one whose backoff is still running", l.Failing)
	}
}

// TestTheSlowLaneIsNotPolled: these change a few times a day.
func TestTheSlowLaneIsNotPolled(t *testing.T) {
	t.Parallel()
	var hits atomic.Int64
	srv := fakeProwlarr(t, &hits)
	s := NewSlow(SlowSources{Prowlarr: arr.New(arr.Prowlarr, srv.URL, "k")})
	now := time.Now()
	s.Read(t.Context(), now)
	first := hits.Load()
	for i := 1; i <= 10; i++ {
		s.Read(t.Context(), now.Add(time.Duration(i)*5*time.Second))
	}
	if hits.Load() != first {
		t.Errorf("asked Prowlarr again within %s: %d → %d requests", slowEvery, first, hits.Load())
	}
	s.Read(t.Context(), now.Add(slowEvery+time.Second))
	if hits.Load() == first {
		t.Error("never refreshed after the interval")
	}
}

func TestBazarrBadgesFromTheRealCapture(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("testdata/bazarr_badges.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	l := NewSlow(SlowSources{Bazarr: bazarr.New(srv.URL, "k")}).Read(t.Context(), time.Now())
	if l.Subs == nil || l.Subs.Episodes != 499 || l.Subs.Movies != 11 || l.Subs.SonarrSignalR != "LIVE" {
		t.Errorf("subs = %+v", l.Subs)
	}
}
