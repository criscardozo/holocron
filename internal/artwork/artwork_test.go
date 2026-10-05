package artwork

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const goodID = "f27caa37e5142225cceded48f6553502"

func TestValidRefusesAnythingButPlainIDs(t *testing.T) {
	cases := []struct {
		kind, id string
		ok       bool
	}{
		{KindJellyfin, goodID, true},
		{KindJellyfin, "F27CAA37E5142225CCEDED48F6553502", false},
		{KindJellyfin, "../../etc/passwd", false},
		{KindJellyfin, goodID + "/x", false},
		{KindTMDb, "kqjL17yufvn9OVLyXYpvtyrFfak.jpg", true},
		{KindTMDb, "a.png", false},
		{KindTMDb, "../x.jpg", false},
		{KindTMDb, "x.jpg.exe", false},
		{"plex", goodID, false},
	}
	for _, c := range cases {
		if got := Valid(c.kind, c.id); got != c.ok {
			t.Errorf("Valid(%q, %q) = %v, want %v", c.kind, c.id, got, c.ok)
		}
	}
	if got := TMDbURL("/kqjL17yufvn9OVLyXYpvtyrFfak.jpg"); got != "/art/tmdb/kqjL17yufvn9OVLyXYpvtyrFfak.jpg" {
		t.Errorf("TMDbURL = %q", got)
	}
	if got := TMDbURL(""); got != "" {
		t.Errorf("TMDbURL of nothing = %q, want empty", got)
	}
}

func TestGetFetchesOnceThenServesFromDisk(t *testing.T) {
	var calls atomic.Int32
	jf := func(_ context.Context, id string, width int) ([]byte, string, error) {
		calls.Add(1)
		if id != goodID || width != Width {
			t.Errorf("fetched %q at %d", id, width)
		}
		return []byte("jpeg"), "image/jpeg", nil
	}
	dir := t.TempDir()
	s, err := New(dir, jf, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			img, err := s.Get(t.Context(), KindJellyfin, goodID)
			if err != nil || string(img.Data) != "jpeg" {
				t.Errorf("Get = %q, %v", img.Data, err)
			}
		})
	}
	wg.Wait()
	if _, err := s.Get(t.Context(), KindJellyfin, goodID); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("source asked %d times, want 1", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "jf-"+goodID+".jpg")); err != nil {
		t.Errorf("not cached on disk: %v", err)
	}
}

func TestMissingPosterIsRememberedForAWhile(t *testing.T) {
	var calls atomic.Int32
	jf := func(context.Context, string, int) ([]byte, string, error) {
		calls.Add(1)
		return nil, "", ErrNotFound
	}
	s, err := New(t.TempDir(), jf, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	s.now = func() time.Time { return now }

	for range 3 {
		if _, err := s.Get(t.Context(), KindJellyfin, goodID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("source asked %d times within the miss window, want 1", n)
	}
	now = now.Add(missTTL + time.Minute)
	_, _ = s.Get(t.Context(), KindJellyfin, goodID)
	if n := calls.Load(); n != 2 {
		t.Errorf("source asked %d times after the window, want 2", n)
	}
}

func TestStalePosterSurvivesTheSourceFailing(t *testing.T) {
	fail := false
	jf := func(context.Context, string, int) ([]byte, string, error) {
		if fail {
			return nil, "", errors.New("jellyfin is restarting")
		}
		return []byte("old"), "image/jpeg", nil
	}
	s, err := New(t.TempDir(), jf, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(t.Context(), KindJellyfin, goodID); err != nil {
		t.Fatal(err)
	}
	fail = true
	later := time.Now().Add(maxAge + time.Hour)
	s.now = func() time.Time { return later }
	img, err := s.Get(t.Context(), KindJellyfin, goodID)
	if err != nil || string(img.Data) != "old" {
		t.Errorf("Get = %q, %v; want the stale copy", img.Data, err)
	}
}

func TestTMDbChecksWhatComesBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/w300/good.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("poster"))
		case "/w300/html.jpg":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	s, err := New(t.TempDir(), nil, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	s.tmdbBase = srv.URL + "/w300/"

	if img, err := s.Get(t.Context(), KindTMDb, "good.jpg"); err != nil || string(img.Data) != "poster" {
		t.Errorf("good: %q, %v", img.Data, err)
	}
	if _, err := s.Get(t.Context(), KindTMDb, "html.jpg"); err == nil {
		t.Error("an HTML answer was accepted as a poster")
	}
	if _, err := s.Get(t.Context(), KindTMDb, "gone.jpg"); !errors.Is(err, ErrNotFound) {
		t.Errorf("gone: %v, want ErrNotFound", err)
	}
}
