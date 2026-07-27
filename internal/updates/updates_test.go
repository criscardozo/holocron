package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cristian/holocron/internal/version"
)

func TestNewer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tag, current string
		want         bool
	}{
		{"v0.3.0", "v0.2.1", true},
		{"v1.0.0", "v0.9.9", true},
		{"v0.2.2", "v0.2.1", true},
		{"v0.2.1", "v0.2.1", false},
		{"v0.2.0", "v0.2.1", false},
		{"v0.10.0", "v0.9.0", true}, // numeric, not lexicographic
		{"0.3.0", "v0.2.1", true},   // a missing "v" still compares
		{"v0.3.0-rc1", "v0.2.1", true},
		// Unparsable input must never claim an update is available.
		{"latest", "v0.2.1", false},
		{"v0.3", "v0.2.1", false},
		{"v0.3.0", "dev", false},
		{"", "v0.2.1", false},
	}
	for _, c := range cases {
		if got := newer(c.tag, c.current); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.tag, c.current, got, c.want)
		}
	}
}

// fakeGitHub serves one release payload and counts the requests, so the cache
// can be observed.
func fakeGitHub(t *testing.T, tag string, calls *int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*calls++
		_, _ = w.Write([]byte(`{
			"tag_name": "` + tag + `",
			"body": "- Algo nuevo",
			"html_url": "https://github.com/criscardozo/holocron/releases/tag/` + tag + `",
			"published_at": "2026-07-26T10:00:00Z"
		}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestService(t *testing.T, url string) *Service {
	t.Helper()
	s := NewService(t.TempDir())
	s.url = url
	s.unitPath = filepath.Join(t.TempDir(), "holocron-update.path")
	return s
}

func TestStatusReportsAnAvailableUpdate(t *testing.T) {
	calls := 0
	srv := fakeGitHub(t, "v9.9.9", &calls)
	s := newTestService(t, srv.URL)

	// Pretend this build came from a tag.
	old := version.Version
	version.Version = "v0.2.1"
	t.Cleanup(func() { version.Version = old })

	st := s.Status(context.Background(), false)
	if !st.Checkable {
		t.Fatal("a tagged build should be checkable")
	}
	if !st.Available {
		t.Errorf("expected an update to be available, got %+v", st)
	}
	if st.Latest != "v9.9.9" || st.Current != "v0.2.1" {
		t.Errorf("unexpected versions: %+v", st)
	}
	if st.Notes == "" || st.URL == "" {
		t.Error("release notes and URL should be surfaced")
	}
}

func TestStatusOnTheLatestVersion(t *testing.T) {
	calls := 0
	srv := fakeGitHub(t, "v0.2.1", &calls)
	s := newTestService(t, srv.URL)

	old := version.Version
	version.Version = "v0.2.1"
	t.Cleanup(func() { version.Version = old })

	if st := s.Status(context.Background(), false); st.Available {
		t.Errorf("no update should be offered when versions match: %+v", st)
	}
}

func TestDevBuildIsNotCheckable(t *testing.T) {
	calls := 0
	srv := fakeGitHub(t, "v9.9.9", &calls)
	s := newTestService(t, srv.URL)

	old := version.Version
	version.Version = "dev"
	t.Cleanup(func() { version.Version = old })

	st := s.Status(context.Background(), false)
	if st.Checkable || st.Available {
		t.Errorf("a dev build has no tag to compare: %+v", st)
	}
	if calls != 0 {
		t.Errorf("a dev build should not call GitHub, got %d calls", calls)
	}
}

func TestStatusIsCachedUnlessForced(t *testing.T) {
	calls := 0
	srv := fakeGitHub(t, "v9.9.9", &calls)
	s := newTestService(t, srv.URL)

	old := version.Version
	version.Version = "v0.2.1"
	t.Cleanup(func() { version.Version = old })

	ctx := context.Background()
	s.Status(ctx, false)
	s.Status(ctx, false)
	if calls != 1 {
		t.Errorf("expected the second check to hit the cache, got %d calls", calls)
	}
	s.Status(ctx, true)
	if calls != 2 {
		t.Errorf("a forced check should refetch, got %d calls", calls)
	}
}

func TestStatusSurfacesAFailedCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	s := newTestService(t, srv.URL)

	old := version.Version
	version.Version = "v0.2.1"
	t.Cleanup(func() { version.Version = old })

	st := s.Status(context.Background(), false)
	if st.Error == "" {
		t.Error("a failed check should be reported, not silently look up to date")
	}
	if st.Available {
		t.Error("a failed check must not claim an update is available")
	}
}

func TestRequestInstallNeedsTheHelper(t *testing.T) {
	t.Parallel()
	s := newTestService(t, "http://example.invalid")

	if s.Installable() {
		t.Fatal("the helper should be absent in this test")
	}
	if err := s.RequestInstall(); err == nil {
		t.Error("requesting an install without the helper should fail")
	}
}

func TestRequestInstallDropsTheTrigger(t *testing.T) {
	t.Parallel()
	s := newTestService(t, "http://example.invalid")

	// Simulate the installer having set up the privileged helper.
	if err := os.WriteFile(s.unitPath, []byte("unit"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !s.Installable() {
		t.Fatal("Installable should be true once the unit exists")
	}
	if err := s.RequestInstall(); err != nil {
		t.Fatalf("RequestInstall: %v", err)
	}

	trigger := filepath.Join(s.stateDir, triggerName)
	if _, err := os.Stat(trigger); err != nil {
		t.Fatalf("the trigger file the helper watches for was not written: %v", err)
	}
}
