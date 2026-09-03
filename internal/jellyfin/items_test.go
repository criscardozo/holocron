package jellyfin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cristian/holocron/internal/db"
	"github.com/cristian/holocron/internal/settings"
)

// newSettingsStore backs the link service with a real store: the device id it
// persists is part of what the header carries.
func newSettingsStore(t *testing.T) *settings.Store {
	t.Helper()
	database, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return settings.NewStore(database)
}

func sub(lang string, external bool) MediaStream {
	return MediaStream{Type: "Subtitle", Language: lang, IsExternal: external}
}

func audio(lang string) MediaStream {
	return MediaStream{Type: "Audio", Language: lang}
}

func TestIsSpanish(t *testing.T) {
	t.Parallel()
	yes := []string{"spa", "es", "esp", "lat", "es-419", "es-ES", "es-mx",
		"castellano", "latino", "Spanish", "spa-419", "esp-latino", "SPA"}
	no := []string{"", "und", "unknown", "none", "mis", "eng", "jpn", "fre", "por"}

	for _, l := range yes {
		if !isSpanish(l) {
			t.Errorf("isSpanish(%q) = false, want true", l)
		}
	}
	for _, l := range no {
		if isSpanish(l) {
			t.Errorf("isSpanish(%q) = true, want false", l)
		}
	}
}

func TestNoLanguageCoversEveryUnknownForm(t *testing.T) {
	t.Parallel()
	// "mis" is the ISO code for multiple languages; it means unknown here.
	for _, l := range []string{"", "  ", "und", "UNKNOWN", "none", "mis"} {
		if !hasNoLanguage(l) {
			t.Errorf("hasNoLanguage(%q) = false, want true", l)
		}
	}
	if hasNoLanguage("spa") {
		t.Error("a real language must not count as unknown")
	}
}

func TestSpanishSubtitleState(t *testing.T) {
	t.Parallel()

	t.Run("spanish audio needs no subtitles", func(t *testing.T) {
		t.Parallel()
		// Counting these as "missing subtitles" is noise: nothing is missing
		// for the viewer.
		it := Item{Streams: []MediaStream{audio("spa"), sub("eng", false)}}
		if got := it.SpanishSubtitleState(); got != SubsNotNeeded {
			t.Errorf("state = %v, want SubsNotNeeded", got)
		}
		if !it.HasSpanishSubtitles() {
			t.Error("an item with Spanish audio counts as covered")
		}
	})

	t.Run("spanish subtitle present", func(t *testing.T) {
		t.Parallel()
		it := Item{Streams: []MediaStream{audio("eng"), sub("spa", true)}}
		if got := it.SpanishSubtitleState(); got != SubsPresent {
			t.Errorf("state = %v, want SubsPresent", got)
		}
	})

	// The real "Espanglish" case: an .srt with no language tag in its filename.
	// Jellyfin indexes it but reports Language: null.
	t.Run("unlabelled subtitle is its own category", func(t *testing.T) {
		t.Parallel()
		it := Item{Streams: []MediaStream{audio("eng"), sub("", true)}}
		if got := it.SpanishSubtitleState(); got != SubsUnknownLanguage {
			t.Errorf("state = %v, want SubsUnknownLanguage", got)
		}
		if it.HasSpanishSubtitles() {
			t.Error("an unlabelled subtitle must not be claimed as Spanish")
		}
	})

	t.Run("nothing at all", func(t *testing.T) {
		t.Parallel()
		it := Item{Streams: []MediaStream{audio("eng"), sub("eng", false)}}
		if got := it.SpanishSubtitleState(); got != SubsMissing {
			t.Errorf("state = %v, want SubsMissing", got)
		}
	})

	// The real "Fast X" case: the .srt is named after the 4K file, so Jellyfin
	// attaches it to that MediaSource only. The title is still covered.
	t.Run("subtitles on one version cover the title", func(t *testing.T) {
		t.Parallel()
		it := Item{
			Path: "/m/Fast X (2023) - 1080p.mp4",
			Sources: []MediaSource{
				{Path: "/m/Fast X (2023) - 1080p.mp4"},
				{Path: "/m/Fast X (2023) - 4K.mkv", Streams: []MediaStream{sub("spa", true)}},
			},
		}
		if got := it.SpanishSubtitleState(); got != SubsPresent {
			t.Errorf("state = %v, want SubsPresent", got)
		}
	})
}

func TestFilePathsCoversEveryVersion(t *testing.T) {
	t.Parallel()

	// Multi-version titles: item.Path names only one file, so acting on it
	// alone would process half of them.
	it := Item{
		Path: "/m/Akira (1988) - 1080p.mp4",
		Sources: []MediaSource{
			{Path: "/m/Akira (1988) - 1080p.mp4"},
			{Path: "/m/Akira (1988) - 4K.mkv"},
		},
	}
	got := it.FilePaths()
	if len(got) != 2 {
		t.Fatalf("FilePaths = %v, want both versions", got)
	}

	// Two versions can share a basename and differ only in container, so
	// deduplication must be by full path.
	same := Item{
		Path: "/m/Ex Machina (2015).mkv",
		Sources: []MediaSource{
			{Path: "/m/Ex Machina (2015).mkv"},
			{Path: "/m/Ex Machina (2015).mp4"},
		},
	}
	if got := same.FilePaths(); len(got) != 2 {
		t.Errorf("FilePaths = %v, want 2 distinct containers", got)
	}
}

func TestOnDiskRejectsGhostEpisodes(t *testing.T) {
	t.Parallel()
	// Jellyfin lists episodes known from metadata providers that were never
	// downloaded. Acting on those chases files that do not exist.
	if (Item{Path: ""}).OnDisk() {
		t.Error("an item with no path is not on disk")
	}
	if !(Item{Path: "/m/x.mkv"}).OnDisk() {
		t.Error("an item with a path is on disk")
	}
}

func TestFolder(t *testing.T) {
	t.Parallel()
	movie := Item{Type: TypeMovie, Path: "/mnt/grande/Museum/Aladin/Aladin.mp4"}
	if got, want := movie.Folder(), "/mnt/grande/Museum/Aladin"; got != want {
		t.Errorf("movie folder = %q, want %q", got, want)
	}
	// Series already point at their directory.
	series := Item{Type: TypeSeries, Path: "/mnt/grande/Anime/Dragon Ball"}
	if got, want := series.Folder(), "/mnt/grande/Anime/Dragon Ball"; got != want {
		t.Errorf("series folder = %q, want %q", got, want)
	}
}

func TestCleanDisplayTitle(t *testing.T) {
	t.Parallel()
	// Real libraries contain a stream title with a form feed embedded.
	if got, want := CleanDisplayTitle("\fSoundHandler"), "SoundHandler"; got != want {
		t.Errorf("CleanDisplayTitle = %q, want %q", got, want)
	}
}

func TestSubtitlePaths(t *testing.T) {
	t.Parallel()
	it := Item{Streams: []MediaStream{
		{Type: "Subtitle", Language: "spa", IsExternal: true, Path: "/m/a.es.srt"},
		{Type: "Subtitle", Language: "eng", IsExternal: false}, // embedded: no path
	}}
	got := it.SubtitlePaths()
	if len(got) != 1 || got[0] != "/m/a.es.srt" {
		t.Errorf("SubtitlePaths = %v, want just the external one", got)
	}
}

// TestClientRepairsStoredAddress covers the install that already saved a bare
// host:port: it must start working on upgrade, without anyone re-saving the
// form.
func TestClientRepairsStoredAddress(t *testing.T) {
	t.Parallel()
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"ServerName":"ObiWan","Version":"10.11.11"}`))
	}))
	defer srv.Close()

	// Strip the scheme the test server hands us, to look like what was stored.
	bare := strings.TrimPrefix(srv.URL, "http://")
	if bare == srv.URL {
		t.Fatalf("expected an http test server, got %q", srv.URL)
	}

	info, err := New(bare, "tok", "dev", "test").Info(t.Context())
	if err != nil {
		t.Fatalf("Info against %q: %v", bare, err)
	}
	if info.Name != "ObiWan" {
		t.Errorf("server name = %q", info.Name)
	}
	if gotPath != "/System/Info" {
		t.Errorf("path = %q, want /System/Info", gotPath)
	}
}

// TestAuthHeaderNeverSendsAnEmptyVersion pins the bug that broke linking on a
// real server: Jellyfin 10.11 answers 400 "Error processing request." to an
// Authorization header with Version="", and the link service used to build its
// client with exactly that.
func TestAuthHeaderNeverSendsAnEmptyVersion(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"", "   "} {
		got := New("http://obiwan:8096", "", "dev-1", version).authHeader()
		if strings.Contains(got, `Version=""`) {
			t.Errorf("version %q produced %q", version, got)
		}
	}
	if got := New("http://obiwan:8096", "", "dev-1", "v0.5.2").authHeader(); !strings.Contains(got, `Version="v0.5.2"`) {
		t.Errorf("a real version must survive: %q", got)
	}
}

// TestLinkServiceSendsTheRunningVersion covers the wiring rather than the
// header: the link service is the one client that was built without a version.
func TestLinkServiceSendsTheRunningVersion(t *testing.T) {
	t.Parallel()
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/QuickConnect/Enabled":
			_, _ = w.Write([]byte("true"))
		case "/QuickConnect/Initiate":
			_, _ = w.Write([]byte(`{"Secret":"s","Code":"123456"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	store := newSettingsStore(t)
	if err := store.Set(t.Context(), settings.KeyJellyfinURL, srv.URL); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLinkService(store).Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if strings.Contains(gotAuth, `Version=""`) || !strings.Contains(gotAuth, "Version=") {
		t.Errorf("Authorization = %q", gotAuth)
	}
}

// TestRedeemSendsTheSecretInTheBody pins the second half of the linking bug:
// Jellyfin 10.11 answers 400 "A non-empty request body is required." when the
// secret is passed in the query string, so the flow failed on its last step,
// after the user had already approved the code.
func TestRedeemSendsTheSecretInTheBody(t *testing.T) {
	t.Parallel()
	var gotBody, gotType, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		gotBody, gotType, gotQuery = string(body), r.Header.Get("Content-Type"), r.URL.RawQuery
		_, _ = w.Write([]byte(`{"AccessToken":"tok","User":{"Id":"u1","Name":"cris","Policy":{"IsAdministrator":true}}}`))
	}))
	defer srv.Close()

	auth, err := New(srv.URL, "", "dev-1", "v0.5.2").RedeemQuickConnect(t.Context(), "S3CR3T")
	if err != nil {
		t.Fatalf("RedeemQuickConnect: %v", err)
	}
	if !strings.Contains(gotBody, `"Secret":"S3CR3T"`) {
		t.Errorf("body = %q, want the secret in it", gotBody)
	}
	if gotType != "application/json" {
		t.Errorf("Content-Type = %q", gotType)
	}
	if gotQuery != "" {
		t.Errorf("query = %q, want the secret out of the URL", gotQuery)
	}
	if auth.Token != "tok" || auth.User != "cris" || !auth.Admin {
		t.Errorf("auth = %+v", auth)
	}
}
