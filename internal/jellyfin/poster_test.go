package jellyfin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPosterAsksForAScaledJPEGAndChecksTheAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("poster requested without the client header")
		}
		switch r.URL.Path {
		case "/Items/abc/Images/Primary":
			q := r.URL.Query()
			if q.Get("fillWidth") != "300" || q.Get("fillHeight") != "450" || q.Get("format") != "Jpg" {
				t.Errorf("query = %v", q)
			}
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg"))
		case "/Items/html/Images/Primary":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "dev", "1")

	data, ct, err := c.Poster(t.Context(), "abc", 300)
	if err != nil || string(data) != "jpeg" || ct != "image/jpeg" {
		t.Errorf("Poster = %q, %q, %v", data, ct, err)
	}
	if _, _, err := c.Poster(t.Context(), "gone", 300); !errors.Is(err, ErrNoImage) {
		t.Errorf("missing: %v, want ErrNoImage", err)
	}
	if _, _, err := c.Poster(t.Context(), "html", 300); err == nil {
		t.Error("an HTML answer was taken for a poster")
	}
}
