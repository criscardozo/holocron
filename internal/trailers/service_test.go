package trailers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/db"
	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/jobs"
)

// fakeTool stands in for yt-dlp: it records what it was asked to download and
// answers searches from a fixed set.
type fakeTool struct {
	results   []Candidate
	searchErr error
	dlErr     error
	// downloads records dir and stem, which is what the naming promise is about.
	downloads []string
}

func (f *fakeTool) Available() bool { return true }
func (f *fakeTool) Path() string    { return "/usr/local/bin/yt-dlp" }
func (f *fakeTool) Version(context.Context) (string, error) {
	return time.Now().Format("2006.01.02"), nil
}
func (f *fakeTool) Search(context.Context, string, int) ([]Candidate, error) {
	return f.results, f.searchErr
}
func (f *fakeTool) Download(_ context.Context, _, dir, stem string) error {
	if f.dlErr != nil {
		return f.dlErr
	}
	f.downloads = append(f.downloads, filepath.Join(dir, stem+"-trailer.mp4"))
	return os.WriteFile(filepath.Join(dir, stem+"-trailer.mp4"), []byte("v"), 0o600)
}

func newTrailerService(t *testing.T, tool Tool) (*Service, *folders.Store, string) {
	t.Helper()
	database, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	store := folders.NewStore(database)
	movies := t.TempDir()
	if _, err := store.Add(t.Context(), "Películas", movies, folders.PurposeMovies); err != nil {
		t.Fatal(err)
	}
	return NewService(store, jobs.NewManager(), tool), store, movies
}

func mkFilm(t *testing.T, movies, folder string, files ...string) {
	t.Helper()
	dir := filepath.Join(movies, folder)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestScanFindsOnlyTheOnesWithoutATrailer, and recognises the convention this
// library actually uses — "<name>-trailer.ext", not "<name>.trailer.ext".
func TestScanFindsOnlyTheOnesWithoutATrailer(t *testing.T) {
	t.Parallel()
	svc, _, movies := newTrailerService(t, &fakeTool{})
	mkFilm(t, movies, "Dune (2021)", "Dune (2021).mkv")
	mkFilm(t, movies, "Heat (1995)", "Heat (1995).mkv", "Heat (1995)-trailer.mp4")
	mkFilm(t, movies, "Arrival (2016)", "Arrival (2016).mkv", "ARRIVAL Official Trailer-trailer.webm")

	films, err := svc.scan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(films) != 1 || films[0].Folder != "Dune (2021)" {
		t.Fatalf("films = %+v", films)
	}
	if films[0].Title != "Dune" || films[0].Year != 2021 {
		t.Errorf("parsed %q (%d)", films[0].Title, films[0].Year)
	}
}

// TestTheFileIsNamedAfterTheFolder is ObiWan's finding turned into a test. The
// library already carries trailers named after the YouTube video title, and
// this is the release that stops adding more.
func TestTheFileIsNamedAfterTheFolder(t *testing.T) {
	t.Parallel()
	tool := &fakeTool{results: []Candidate{
		{ID: "a", Title: "DUNE  Trailer oficial  Subtitulos Español Latinoamericano", Duration: 150},
	}}
	svc, _, movies := newTrailerService(t, tool)
	mkFilm(t, movies, "Dune (2021)", "Dune (2021).mkv")

	films, err := svc.scan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.fetch(t.Context(), films, &jobs.Progress{}); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(movies, "Dune (2021)", "Dune (2021)-trailer.mp4")
	if len(tool.downloads) != 1 || tool.downloads[0] != want {
		t.Errorf("downloaded %v, want %q", tool.downloads, want)
	}
}

// TestABrokenExtractorStopsTheWholeBatch. Without this, a stale yt-dlp produces
// 181 identical failures and the summary buries the one thing worth reading.
func TestABrokenExtractorStopsTheWholeBatch(t *testing.T) {
	t.Parallel()
	tool := &fakeTool{searchErr: ErrExtractorBroken}
	svc, _, movies := newTrailerService(t, tool)
	for _, n := range []string{"Dune (2021)", "Heat (1995)", "Arrival (2016)"} {
		mkFilm(t, movies, n, n+".mkv")
	}

	films, err := svc.scan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(films) != 3 {
		t.Fatalf("expected three films, got %d", len(films))
	}
	if _, err := svc.fetch(t.Context(), films, &jobs.Progress{}); err != nil {
		t.Fatal(err)
	}
	if got := len(svc.Results()); got != 1 {
		t.Errorf("kept going after the extractor broke: %d results", got)
	}
	if r := svc.Results()[0]; r.Err == "" || r.Err == "falló la descarga" {
		t.Errorf("the reason is not specific enough: %q", r.Err)
	}
}

// TestFetchOnlyTouchesFoldersItFoundItself. The names come back from a form, so
// this is the containment check: matching against the scan means there is no
// path arithmetic between a request and a write.
func TestFetchOnlyTouchesFoldersItFoundItself(t *testing.T) {
	t.Parallel()
	tool := &fakeTool{results: []Candidate{{ID: "a", Title: "Dune - Official Trailer", Duration: 150}}}
	svc, _, movies := newTrailerService(t, tool)
	mkFilm(t, movies, "Dune (2021)", "Dune (2021).mkv")

	if err := svc.StartScan(t.Context()); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, svc)

	outside := filepath.Join(t.TempDir(), "Privado")
	if err := os.MkdirAll(outside, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := svc.StartFetch(t.Context(), []string{outside, "/etc", "../../tmp"}); err == nil {
		t.Fatal("accepted folders it never scanned")
	}
	if len(tool.downloads) != 0 {
		t.Errorf("downloaded into %v", tool.downloads)
	}
}

// TestNothingIsWrittenWhenNoTrailerIsFound: the film keeps no file at all,
// rather than an empty one or the wrong film's.
func TestNothingIsWrittenWhenNoTrailerIsFound(t *testing.T) {
	t.Parallel()
	tool := &fakeTool{results: []Candidate{
		{ID: "x", Title: "Top Gun Maverick - Official Trailer", Duration: 150},
	}}
	svc, _, movies := newTrailerService(t, tool)
	mkFilm(t, movies, "Como agua para chocolate (1992)", "pelicula.mkv")

	films, err := svc.scan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.fetch(t.Context(), films, &jobs.Progress{}); err != nil {
		t.Fatal(err)
	}
	if len(tool.downloads) != 0 {
		t.Errorf("downloaded a different film's trailer: %v", tool.downloads)
	}
	entries, err := os.ReadDir(filepath.Join(movies, "Como agua para chocolate (1992)"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("something was written: %d entries", len(entries))
	}
}

func waitIdle(t *testing.T, svc *Service) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !svc.Scanning() && !svc.Fetching() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job never finished")
}
