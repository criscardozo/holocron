package torrents

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/db"
	"github.com/cristian/holocron/internal/settings"
)

// fakeQbit counts logins and category listings, which is what the caching in
// this package is about.
type fakeQbit struct {
	logins     atomic.Int64
	categories atomic.Int64
}

func (f *fakeQbit) start(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		f.logins.Add(1)
		_, _ = w.Write([]byte("Ok."))
	})
	mux.HandleFunc("/api/v2/torrents/categories", func(w http.ResponseWriter, _ *http.Request) {
		f.categories.Add(1)
		_, _ = w.Write([]byte(`{"pelis":{"name":"pelis","savePath":"/media/peliculas"}}`))
	})
	mux.HandleFunc("/api/v2/torrents/info", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newService(t *testing.T, url, user, pass string) (*Service, *settings.Store) {
	t.Helper()
	database, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	store := settings.NewStore(database)
	for key, value := range map[string]string{
		settings.KeyQbitURL:  url,
		settings.KeyQbitUser: user,
		settings.KeyQbitPass: pass,
	} {
		if value == "" {
			continue
		}
		if err := store.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	return NewService(store), store
}

// TestClientIsReusedAcrossCalls pins the fix that took qBittorrent from 80
// requests a minute down to 20: the client holds the session cookie, so
// rebuilding it per call meant logging in again every time.
func TestClientIsReusedAcrossCalls(t *testing.T) {
	t.Parallel()
	fake := &fakeQbit{}
	srv := fake.start(t)
	svc, _ := newService(t, srv.URL, "admin", "secret")

	for range 5 {
		if _, err := svc.List(t.Context()); err != nil {
			t.Fatalf("List: %v", err)
		}
	}
	if got := fake.logins.Load(); got != 1 {
		t.Errorf("%d logins for 5 polls, want 1", got)
	}
}

// TestChangingTheCredentialsInvalidatesTheClient: the cache key includes the
// credentials, so editing them in Ajustes has to take effect immediately rather
// than after a restart.
func TestChangingTheCredentialsInvalidatesTheClient(t *testing.T) {
	t.Parallel()
	fake := &fakeQbit{}
	srv := fake.start(t)
	svc, store := newService(t, srv.URL, "admin", "old")

	if _, err := svc.List(t.Context()); err != nil {
		t.Fatalf("List: %v", err)
	}
	if err := store.Set(t.Context(), settings.KeyQbitPass, "new"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.List(t.Context()); err != nil {
		t.Fatalf("List after the password changed: %v", err)
	}

	if got := fake.logins.Load(); got != 2 {
		t.Errorf("%d logins, want 2: the new password must be used at once", got)
	}
}

// TestCategoriesAreCachedBriefly: the add-magnet form asks for categories every
// time it renders, and they change about never.
func TestCategoriesAreCachedBriefly(t *testing.T) {
	t.Parallel()
	fake := &fakeQbit{}
	srv := fake.start(t)
	svc, _ := newService(t, srv.URL, "admin", "secret")

	for range 4 {
		cats, err := svc.Categories(t.Context())
		if err != nil {
			t.Fatalf("Categories: %v", err)
		}
		if len(cats) != 1 || cats[0].Name != "pelis" {
			t.Fatalf("got %+v", cats)
		}
	}
	if got := fake.categories.Load(); got != 1 {
		t.Errorf("%d category listings for 4 renders, want 1", got)
	}

	// Expiring the cache by hand rather than sleeping a minute.
	svc.mu.Lock()
	svc.catsAt = time.Now().Add(-2 * time.Minute)
	svc.mu.Unlock()

	if _, err := svc.Categories(t.Context()); err != nil {
		t.Fatalf("Categories after expiry: %v", err)
	}
	if got := fake.categories.Load(); got != 2 {
		t.Errorf("%d listings, want 2 once the cache went stale", got)
	}
}

func TestUnconfiguredIsItsOwnError(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t, "", "", "")

	if svc.Configured(t.Context()) {
		t.Error("nothing is configured")
	}
	// A distinct error, so the UI can say "configure qBittorrent" instead of
	// reporting a connection failure to a server that was never set.
	if _, err := svc.List(t.Context()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("List = %v, want ErrNotConfigured", err)
	}
	if _, err := svc.Categories(t.Context()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Categories = %v, want ErrNotConfigured", err)
	}
	if err := svc.Delete(t.Context(), "abc"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Delete = %v, want ErrNotConfigured", err)
	}
}
