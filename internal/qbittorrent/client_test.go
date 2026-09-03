package qbittorrent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientFlow(t *testing.T) {
	t.Parallel()
	var addedMagnet string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "abc", Path: "/"})
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("/api/v2/torrents/info", func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("SID"); err != nil {
			t.Error("missing session cookie on info")
		}
		_, _ = w.Write([]byte(`[{"hash":"h1","name":"Ubuntu ISO","state":"downloading",
			"progress":0.42,"size":1000,"dlspeed":2048,"upspeed":128,"num_seeds":5,"num_leechs":2}]`))
	})
	mux.HandleFunc("/api/v2/torrents/add", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		addedMagnet = r.PostFormValue("urls")
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("/api/v2/torrents/pause", func(w http.ResponseWriter, _ *http.Request) {})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	c, err := New(srv.URL, "admin", "pass")
	if err != nil {
		t.Fatal(err)
	}

	list, err := c.Torrents(ctx)
	if err != nil {
		t.Fatalf("Torrents: %v", err)
	}
	if len(list) != 1 || list[0].Name != "Ubuntu ISO" || list[0].DlSpeed != 2048 {
		t.Fatalf("unexpected torrents: %+v", list)
	}

	if err := c.AddMagnet(ctx, "magnet:?xt=urn:btih:demo", ""); err != nil {
		t.Fatalf("AddMagnet: %v", err)
	}
	if addedMagnet != "magnet:?xt=urn:btih:demo" {
		t.Errorf("added magnet = %q", addedMagnet)
	}

	if err := c.Pause(ctx, "h1"); err != nil {
		t.Errorf("Pause: %v", err)
	}
}

func TestCategories(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "abc", Path: "/"})
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("/api/v2/torrents/categories", func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("SID"); err != nil {
			t.Error("missing session cookie on categories")
		}
		// qBittorrent returns an unordered object keyed by name; older builds
		// leave the inner "name" empty.
		_, _ = w.Write([]byte(`{
			"Series":{"name":"Series","savePath":"/downloads/series"},
			"Peliculas":{"name":"Peliculas","savePath":"/downloads/pelis"},
			"Sin nombre":{"savePath":"/downloads/otros"}
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := New(srv.URL, "admin", "pass")
	if err != nil {
		t.Fatal(err)
	}
	cats, err := c.Categories(context.Background())
	if err != nil {
		t.Fatalf("Categories: %v", err)
	}
	if len(cats) != 3 {
		t.Fatalf("got %d categories, want 3: %+v", len(cats), cats)
	}
	// Sorted by name, so the UI order does not jump around between refreshes.
	want := []string{"Peliculas", "Series", "Sin nombre"}
	for i, name := range want {
		if cats[i].Name != name {
			t.Errorf("category %d = %q, want %q", i, cats[i].Name, name)
		}
	}
	if cats[0].SavePath != "/downloads/pelis" {
		t.Errorf("save path = %q", cats[0].SavePath)
	}
}

func TestCategoriesWhenNoneAreConfigured(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "abc", Path: "/"})
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("/api/v2/torrents/categories", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := New(srv.URL, "admin", "pass")
	if err != nil {
		t.Fatal(err)
	}
	cats, err := c.Categories(context.Background())
	if err != nil {
		t.Fatalf("Categories: %v", err)
	}
	if len(cats) != 0 {
		t.Errorf("expected no categories, got %+v", cats)
	}
}

func TestAddMagnetSendsTheCategory(t *testing.T) {
	t.Parallel()
	var gotCategory, gotMagnet string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "abc", Path: "/"})
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("/api/v2/torrents/add", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotMagnet = r.PostFormValue("urls")
		gotCategory = r.PostFormValue("category")
		_, _ = w.Write([]byte("Ok."))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := New(srv.URL, "admin", "pass")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.AddMagnet(ctx, "magnet:?xt=urn:btih:abc", "Peliculas"); err != nil {
		t.Fatalf("AddMagnet: %v", err)
	}
	if gotMagnet != "magnet:?xt=urn:btih:abc" {
		t.Errorf("magnet = %q", gotMagnet)
	}
	if gotCategory != "Peliculas" {
		t.Errorf("category = %q, want Peliculas", gotCategory)
	}

	// No category means the field is omitted, not sent empty.
	gotCategory = "unset"
	if err := c.AddMagnet(ctx, "magnet:?xt=urn:btih:def", ""); err != nil {
		t.Fatalf("AddMagnet without category: %v", err)
	}
	if gotCategory != "" {
		t.Errorf("category = %q, want it absent", gotCategory)
	}
}

// The session cookie must be reused: the torrents page polls every few seconds,
// and logging in on every call hammered qBittorrent with pointless auth traffic.
func TestSessionIsReusedAcrossCalls(t *testing.T) {
	t.Parallel()
	logins := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		logins++
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "abc", Path: "/"})
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("/api/v2/torrents/info", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	mux.HandleFunc("/api/v2/torrents/categories", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/api/v2/torrents/pause", func(http.ResponseWriter, *http.Request) {})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := New(srv.URL, "admin", "pass")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for range 5 {
		if _, err := c.Torrents(ctx); err != nil {
			t.Fatalf("Torrents: %v", err)
		}
		if _, err := c.Categories(ctx); err != nil {
			t.Fatalf("Categories: %v", err)
		}
	}
	if err := c.Pause(ctx, "h1"); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if logins != 1 {
		t.Errorf("logged in %d times across 11 calls, want 1", logins)
	}
}

// A long-lived client outlives qBittorrent's session, so an expired one must be
// refreshed transparently instead of surfacing as an error.
func TestExpiredSessionIsRetriedOnGet(t *testing.T) {
	t.Parallel()
	logins, infoCalls := 0, 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		logins++
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "fresh", Path: "/"})
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("/api/v2/torrents/info", func(w http.ResponseWriter, _ *http.Request) {
		infoCalls++
		if infoCalls == 1 {
			// First call: qBittorrent has forgotten the session.
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`[{"hash":"h1","name":"A","state":"downloading",
			"progress":0.1,"size":1,"dlspeed":0,"upspeed":0,"num_seeds":0,"num_leechs":0}]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := New(srv.URL, "admin", "pass")
	if err != nil {
		t.Fatal(err)
	}
	list, err := c.Torrents(context.Background())
	if err != nil {
		t.Fatalf("Torrents should recover from an expired session: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d torrents after the retry, want 1", len(list))
	}
	if logins != 2 {
		t.Errorf("logged in %d times, want 2 (initial + refresh)", logins)
	}
}

// TestClientRepairsStoredAddress covers the install that saved a bare
// host:port in the settings form: it has to work, because net/url cannot parse
// that as a URL and the failure would look like qBittorrent being down.
func TestClientRepairsStoredAddress(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte("[]"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	bare := strings.TrimPrefix(srv.URL, "http://")
	if bare == srv.URL {
		t.Fatalf("expected an http test server, got %q", srv.URL)
	}

	c, err := New(bare, "admin", "pass")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.Torrents(t.Context()); err != nil {
		t.Fatalf("Torrents against %q: %v", bare, err)
	}
}
