package httpserver

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/activity"
	"github.com/cristian/holocron/internal/apitoken"
	"github.com/cristian/holocron/internal/artwork"
	"github.com/cristian/holocron/internal/db"
	"github.com/cristian/holocron/internal/diskusage"
	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/hardware"
	"github.com/cristian/holocron/internal/jellyfin"
	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/library"
	"github.com/cristian/holocron/internal/live"
	"github.com/cristian/holocron/internal/naming"
	"github.com/cristian/holocron/internal/power"
	"github.com/cristian/holocron/internal/quality"
	"github.com/cristian/holocron/internal/services"
	"github.com/cristian/holocron/internal/settings"
	"github.com/cristian/holocron/internal/torrents"
	"github.com/cristian/holocron/internal/updates"
	"github.com/cristian/holocron/internal/widgets"
)

// testServer is a fully wired server over a temporary database, which is what
// the HTTP layer needs to be exercised at all: nearly every handler reaches a
// service, and the services reach SQLite.
type testServer struct {
	*httptest.Server
	deps Deps
	logs *strings.Builder
	db   *sql.DB
}

// testPosterID is the one poster the test server's fake Jellyfin has.
const testPosterID = "f27caa37e5142225cceded48f6553502"

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	ctx := context.Background()

	database, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	// Logs are captured rather than discarded: a few of these behaviours are
	// only observable as a log line, such as a rejected update request.
	logs := &strings.Builder{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	jobManager := jobs.NewManager()
	folderStore := folders.NewStore(database)
	settingsStore := settings.NewStore(database)

	deps := Deps{
		Log:      logger,
		Folders:  folderStore,
		Disk:     diskusage.NewService(database, folderStore, jobManager),
		Hardware: hardware.NewHub(20 * time.Millisecond),
		Services: live.NewHub(20*time.Millisecond, services.NewReader(services.Config{}).Read),
		Activity: live.NewHub(20*time.Millisecond, activity.NewSampler(func(context.Context) activity.Sources {
			return activity.Sources{}
		}).Sample),
		Naming:       naming.NewService(database, folderStore),
		Settings:     settingsStore,
		Library:      library.NewService(database, settingsStore, jobManager),
		Quality:      quality.NewService(database, settingsStore, jobManager),
		Torrents:     torrents.NewService(settingsStore),
		APIToken:     apitoken.NewStore(settingsStore),
		JellyfinLink: jellyfin.NewLinkService(settingsStore),
		Updates:      updates.NewService(t.TempDir()),
		Power:        power.NewService(t.TempDir()),
	}
	art, err := artwork.New(t.TempDir(), func(_ context.Context, id string, _ int) ([]byte, string, error) {
		if id == testPosterID {
			return []byte("jpeg"), "image/jpeg", nil
		}
		return nil, "", artwork.ErrNotFound
	}, logger)
	if err != nil {
		t.Fatalf("artwork: %v", err)
	}
	deps.Art = art
	deps.Widgets = widgets.NewRegistry(
		widgets.SystemWidget{},
		widgets.NewDiskWidget(folderStore),
		widgets.NewNamingWidget(deps.Naming),
		widgets.NewMediaWidget(deps.Library),
		widgets.NewQualityWidget(deps.Quality),
		widgets.NewTorrentsWidget(deps.Torrents),
	)

	srv := httptest.NewServer(New(deps).Handler())
	t.Cleanup(srv.Close)
	return &testServer{Server: srv, deps: deps, logs: logs, db: database}
}

// response is the whole answer, already read and closed. Returning this rather
// than an *http.Response means no test can leak a body, and the assertions read
// better for it.
type response struct {
	Status int
	Header http.Header
	Body   string
}

// do sends a request without following redirects, so a 303 is observable.
func (ts *testServer) do(t *testing.T, req *http.Request) response {
	t.Helper()
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body of %s %s: %v", req.Method, req.URL.Path, err)
	}
	return response{Status: resp.StatusCode, Header: resp.Header, Body: string(body)}
}

func (ts *testServer) post(t *testing.T, path string, form url.Values, headers map[string]string) response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		ts.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return ts.do(t, req)
}

func (ts *testServer) get(t *testing.T, path string, headers map[string]string) response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return ts.do(t, req)
}

func (ts *testServer) folderCount(t *testing.T) int {
	t.Helper()
	list, err := ts.deps.Folders.List(t.Context(), "")
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	return len(list)
}
