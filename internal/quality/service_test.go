package quality

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/db"
	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/settings"
)

// fakeJellyfin serves total items in pages, recording what it was asked and
// whether a refresh was requested.
type fakeJellyfin struct {
	pages     int
	refreshed []string
}

func (f *fakeJellyfin) start(t *testing.T, total int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/Items", func(w http.ResponseWriter, r *http.Request) {
		f.pages++
		q := r.URL.Query()
		if q.Get("userId") != "" {
			t.Errorf("the audit must not scope the listing to a user: %q", r.URL.RawQuery)
		}
		if !strings.Contains(q.Get("IncludeItemTypes"), "Episode") {
			t.Errorf("the audit must include episodes, got %q", q.Get("IncludeItemTypes"))
		}
		if !strings.Contains(q.Get("Fields"), "Overview") {
			t.Errorf("the audit needs Overview, got %q", q.Get("Fields"))
		}

		start, _ := strconv.Atoi(q.Get("StartIndex"))
		limit, _ := strconv.Atoi(q.Get("Limit"))
		items := make([]map[string]any, 0, limit)
		for i := start; i < min(start+limit, total); i++ {
			items = append(items, map[string]any{
				"Id": "ep-" + strconv.Itoa(i), "Type": "Episode",
				"Name":       "Episode " + strconv.Itoa(i), // generic on purpose
				"Path":       "/media/series/Show/ep" + strconv.Itoa(i) + ".mkv",
				"SeriesName": "Show", "SeriesId": "show-1",
				"ParentIndexNumber": 1, "IndexNumber": i,
				"MediaStreams": []any{map[string]any{"Type": "Audio", "Language": "eng"}},
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"TotalRecordCount": total, "Items": items,
		})
	})

	mux.HandleFunc("POST /Items/{id}/Refresh", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("metadataRefreshMode"); got != "FullRefresh" {
			t.Errorf("refresh mode = %q, want FullRefresh (Default was measured not to write)", got)
		}
		f.refreshed = append(f.refreshed, r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newService(t *testing.T, url string, admin bool) (*Service, context.Context) {
	t.Helper()
	ctx := context.Background()
	database, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	store := settings.NewStore(database)
	for k, v := range map[string]string{
		settings.KeyJellyfinURL:      url,
		settings.KeyJellyfinToken:    "tok",
		settings.KeyJellyfinDeviceID: "dev",
		settings.KeyJellyfinAdmin:    strconv.FormatBool(admin),
	} {
		if err := store.Set(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}
	return NewService(database, store, jobs.NewManager()), ctx
}

func waitForJob(t *testing.T, svc *Service, id string) {
	t.Helper()
	for range 300 {
		if job, ok := svc.jobs.Get(id); ok && job.Status != jobs.StatusRunning {
			if job.Status == jobs.StatusError {
				t.Fatalf("audit failed: %s", job.Err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the audit did not finish")
}

func TestScanPagesAndCaches(t *testing.T) {
	t.Parallel()
	// Comfortably more than one page: the audit has to page, because episodes
	// carry their streams and asking for two thousand at once is a
	// multi-megabyte decode on a Pi.
	const total = 1000

	fake := &fakeJellyfin{}
	srv := fake.start(t, total)
	svc, ctx := newService(t, srv.URL, true)

	report, ok, err := svc.Latest(ctx)
	if err != nil {
		t.Fatalf("Latest before any scan should be empty, not an error: %v", err)
	}
	if ok {
		t.Fatalf("there is no report before the first scan, got %+v", report)
	}

	job, err := svc.StartScan(ctx)
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	waitForJob(t, svc, job.ID)

	if fake.pages < 2 {
		t.Errorf("the audit made %d request(s); %d items should not fit in one page", fake.pages, total)
	}

	report, ok, err = svc.Latest(ctx)
	if err != nil || !ok {
		t.Fatalf("Latest = %v, %v", ok, err)
	}
	if report.Scanned != total {
		t.Errorf("scanned = %d, want %d (every page must be kept)", report.Scanned, total)
	}
	// English audio, no subtitles, and "Episode N" for a title.
	if got := report.Count(CatSubsMissing); got != total {
		t.Errorf("subs-missing = %d, want %d", got, total)
	}
	if got := report.Count(CatGenericTitle); got != total {
		t.Errorf("generic-title = %d, want %d", got, total)
	}
	if report.GeneratedAt.IsZero() {
		t.Error("the report must be stamped, the page shows when it ran")
	}

	// A second scan replaces the first rather than accumulating.
	job2, err := svc.StartScan(ctx)
	if err != nil {
		t.Fatalf("second StartScan: %v", err)
	}
	waitForJob(t, svc, job2.ID)
	if again, _, _ := svc.Latest(ctx); again.Scanned != total {
		t.Errorf("scanned after rescan = %d, want %d", again.Scanned, total)
	}
}

func TestRefreshOnlyTouchesReportedItems(t *testing.T) {
	t.Parallel()
	fake := &fakeJellyfin{}
	srv := fake.start(t, 3)
	svc, ctx := newService(t, srv.URL, true)

	job, err := svc.StartScan(ctx)
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	waitForJob(t, svc, job.ID)

	if err := svc.Refresh(ctx, "ep-1"); err != nil {
		t.Fatalf("refreshing a reported item: %v", err)
	}
	if len(fake.refreshed) != 1 || fake.refreshed[0] != "ep-1" {
		t.Fatalf("refreshed = %v, want [ep-1]", fake.refreshed)
	}

	// The id comes from the browser and ends in a write on the media server, so
	// anything the report never saw is refused before the request is made.
	for _, bogus := range []string{"", "   ", "ep-999", "../../System/Shutdown"} {
		if err := svc.Refresh(ctx, bogus); !errors.Is(err, ErrUnknownItem) {
			t.Errorf("Refresh(%q) = %v, want ErrUnknownItem", bogus, err)
		}
	}
	if len(fake.refreshed) != 1 {
		t.Errorf("a rejected id still reached Jellyfin: %v", fake.refreshed)
	}
}

func TestRefreshNeedsAnAdministrator(t *testing.T) {
	t.Parallel()
	fake := &fakeJellyfin{}
	srv := fake.start(t, 2)
	svc, ctx := newService(t, srv.URL, false)

	job, err := svc.StartScan(ctx)
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	waitForJob(t, svc, job.ID)

	// Jellyfin answers 403 for a non-admin. Saying so up front beats letting
	// the user press a button that cannot work.
	if err := svc.Refresh(ctx, "ep-0"); !errors.Is(err, ErrNotAdmin) {
		t.Fatalf("Refresh as a non-admin = %v, want ErrNotAdmin", err)
	}
	if len(fake.refreshed) != 0 {
		t.Errorf("the request was made anyway: %v", fake.refreshed)
	}
}

func TestScanNeedsALinkedServer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = database.Close() }()

	svc := NewService(database, settings.NewStore(database), jobs.NewManager())
	if svc.Configured(ctx) {
		t.Error("nothing is linked yet")
	}
	if _, err := svc.StartScan(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("StartScan = %v, want ErrNotConfigured", err)
	}
}
