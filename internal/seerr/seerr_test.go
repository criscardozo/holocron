package seerr

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// The request fixture is a real capture from Ginebra with the requesters'
// emails and avatars removed on the server before it left.
func fakeSeerr(t *testing.T, titleCalls *atomic.Int64) *httptest.Server {
	t.Helper()
	reqs, err := os.ReadFile("testdata/requests.json")
	if err != nil {
		t.Fatal(err)
	}
	count, err := os.ReadFile("testdata/count.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/v1/request":
			_, _ = w.Write(reqs)
		case r.URL.Path == "/api/v1/request/count":
			_, _ = w.Write(count)
		case strings.HasPrefix(r.URL.Path, "/api/v1/movie/"):
			titleCalls.Add(1)
			_, _ = w.Write([]byte(`{"title":"Película ` + strings.TrimPrefix(r.URL.Path, "/api/v1/movie/") + `","releaseDate":"2020-01-26","posterPath":"/kqjL17yufvn9OVLyXYpvtyrFfak.jpg"}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/tv/"):
			titleCalls.Add(1)
			_, _ = w.Write([]byte(`{"name":"Serie","firstAirDate":"2023-05-18"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRecentRequestsCarryTheirTitles(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	c := New(fakeSeerr(t, &calls).URL, "k")

	reqs, err := c.Recent(t.Context(), 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) == 0 {
		t.Fatal("no requests from a capture that has them")
	}
	for _, r := range reqs {
		if r.Title == "" || strings.HasPrefix(r.Title, "TMDb ") {
			t.Errorf("request %d has no title: %+v", r.ID, r)
		}
		if r.Type == "movie" && r.Poster != "/kqjL17yufvn9OVLyXYpvtyrFfak.jpg" {
			t.Errorf("request %d lost its poster: %q", r.ID, r.Poster)
		}
		if strings.Contains(r.By, "@") {
			t.Errorf("an email address made it into the requester: %q", r.By)
		}
	}
	first := calls.Load()

	// Titles never change for a TMDb id, so a second read costs no lookups.
	if _, err := c.Recent(t.Context(), 6); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != first {
		t.Errorf("looked titles up again: %d then %d", first, calls.Load())
	}
}

func TestCountMatchesTheCapture(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	n, err := New(fakeSeerr(t, &calls).URL, "k").Count(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if n.Total != 21 || n.Pending != 0 {
		t.Errorf("counts = %+v", n)
	}
}
