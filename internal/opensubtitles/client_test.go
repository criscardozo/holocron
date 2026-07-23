package opensubtitles

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchLoginDownload(t *testing.T) {
	t.Parallel()
	var srv *httptest.Server
	mux := http.NewServeMux()

	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Api-Key") == "" {
			t.Error("missing Api-Key header on login")
		}
		_, _ = w.Write([]byte(`{"token":"tok123"}`))
	})
	mux.HandleFunc("/subtitles", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("languages") != "es" {
			t.Errorf("languages = %q, want es", r.URL.Query().Get("languages"))
		}
		_, _ = w.Write([]byte(`{"data":[{"attributes":{"language":"es","release":"BluRay",
			"files":[{"file_id":42,"file_name":"movie.es.srt"}],
			"feature_details":{"title":"The Matrix","year":1999}}}]}`))
	})
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok123" {
			t.Errorf("Authorization = %q, want Bearer tok123", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"link":"` + srv.URL + `/dl/movie.es.srt","file_name":"movie.es.srt"}`))
	})
	mux.HandleFunc("/dl/movie.es.srt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n"))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	c := New("key").WithBaseURL(srv.URL)

	if err := c.Login(ctx, "user", "pass"); err != nil {
		t.Fatalf("Login: %v", err)
	}

	subs, err := c.Search(ctx, "The Matrix", 1999, "es")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(subs) != 1 || subs[0].FileID != 42 {
		t.Fatalf("unexpected search results: %+v", subs)
	}

	content, name, err := c.Download(ctx, subs[0].FileID)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if name != "movie.es.srt" {
		t.Errorf("filename = %q, want movie.es.srt", name)
	}
	if !strings.Contains(string(content), "Hola") {
		t.Errorf("content missing expected text; got %q", content)
	}
}
