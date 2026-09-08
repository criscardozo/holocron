package trailers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
func (f *fakeTool) Download(_ context.Context, _, dir, stem string, _ int) error {
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

// pickyTool fails the first N downloads with err, then succeeds. Stands in for
// the two things a search cannot see: a video below the resolution floor and
// one YouTube has since removed.
type pickyTool struct {
	fakeTool
	failFirst int
	failWith  error
	attempts  []string
	floors    []int
}

func (p *pickyTool) Download(ctx context.Context, url, dir, stem string, floor int) error {
	p.attempts = append(p.attempts, url)
	p.floors = append(p.floors, floor)
	if len(p.attempts) <= p.failFirst {
		return p.failWith
	}
	return p.fakeTool.Download(ctx, url, dir, stem, floor)
}

// TestALowResCandidateFallsThroughToTheNext. The library already holds trailers
// at 320x240 and 450x360: those pass every filter based on duration and title
// and are useless on a television. The floor cannot be applied during the
// search — a flat search does not report resolution and asking costs fifteen
// times as long — so it lands here, and it has to leave a way forward.
func TestALowResCandidateFallsThroughToTheNext(t *testing.T) {
	t.Parallel()
	tool := &pickyTool{
		fakeTool: fakeTool{results: []Candidate{
			{ID: "small", Title: "Antes de amanecer - Trailer oficial", Duration: 95},
			{ID: "good", Title: "Antes de amanecer (1995) - Trailer subtitulado", Duration: 122},
		}},
		failFirst: 1,
		failWith:  ErrTooLowRes,
	}
	svc, _, movies := newTrailerService(t, tool)
	mkFilm(t, movies, "Antes de amanecer (1995)", "pelicula.mkv")

	films, err := svc.scan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.fetch(t.Context(), films, &jobs.Progress{}); err != nil {
		t.Fatal(err)
	}
	if len(tool.attempts) != 2 {
		t.Fatalf("tried %d candidates, want 2: %v", len(tool.attempts), tool.attempts)
	}
	res := svc.Results()
	if len(res) != 1 || res[0].Err != "" {
		t.Fatalf("results = %+v", res)
	}
	if res[0].Trailer == "" {
		t.Error("nothing was recorded as downloaded")
	}
}

// TestADeletedVideoAlsoFallsThrough, for the same reason: it is a property of
// that one candidate, not of the film.
func TestADeletedVideoAlsoFallsThrough(t *testing.T) {
	t.Parallel()
	tool := &pickyTool{
		fakeTool: fakeTool{results: []Candidate{
			{ID: "gone", Title: "Heat (1995) - Official Trailer", Duration: 140},
			{ID: "ok", Title: "Heat 1995 Trailer subtitulado", Duration: 135},
		}},
		failFirst: 1,
		failWith:  ErrGone,
	}
	svc, _, movies := newTrailerService(t, tool)
	mkFilm(t, movies, "Heat (1995)", "pelicula.mkv")

	films, _ := svc.scan(t.Context())
	if _, err := svc.fetch(t.Context(), films, &jobs.Progress{}); err != nil {
		t.Fatal(err)
	}
	if len(tool.attempts) != 2 {
		t.Errorf("tried %v", tool.attempts)
	}
}

// TestGivingUpOnAFilmDoesNotGiveUpOnTheBatch. When every candidate for one film
// is too small, the next film still gets its turn — unlike a broken extractor,
// which stops everything because it will repeat.
func TestGivingUpOnAFilmDoesNotGiveUpOnTheBatch(t *testing.T) {
	t.Parallel()
	tool := &pickyTool{
		fakeTool: fakeTool{results: []Candidate{
			{ID: "a", Title: "Dune - Official Trailer", Duration: 150},
		}},
		failFirst: 99,
		failWith:  ErrTooLowRes,
	}
	svc, _, movies := newTrailerService(t, tool)
	mkFilm(t, movies, "Dune (2021)", "p.mkv")
	mkFilm(t, movies, "Heat (1995)", "p.mkv")

	films, _ := svc.scan(t.Context())
	if _, err := svc.fetch(t.Context(), films, &jobs.Progress{}); err != nil {
		t.Fatal(err)
	}
	res := svc.Results()
	if len(res) != 2 {
		t.Fatalf("stopped after %d films, want both", len(res))
	}
	// Dune is the one the candidate matches, so it is the one that got as far
	// as a download and hit the floor. Heat is refused earlier, by the title
	// check, and says so differently — which is the point: the two failures
	// are not the same and must not read as if they were.
	var dune, heat string
	for _, r := range res {
		if strings.HasPrefix(r.Folder, "Dune") {
			dune = r.Err
		} else {
			heat = r.Err
		}
	}
	if !strings.Contains(dune, "360p") {
		t.Errorf("Dune should name the resolution floor, got %q", dune)
	}
	if !strings.Contains(heat, "no se encontró") {
		t.Errorf("Heat should say nothing matched, got %q", heat)
	}
	// One candidate, tried once at each floor: the preferred one and then the
	// hard one. maxAttempts caps candidates, not passes.
	if len(tool.attempts) != 2 {
		t.Errorf("attempted %d downloads for one candidate: %v", len(tool.attempts), tool.attempts)
	}
	if len(tool.floors) != 2 || tool.floors[0] <= tool.floors[1] {
		t.Errorf("floors = %v, want the preferred one attempted before the hard one", tool.floors)
	}
}

// TestAHigherResCandidateWinsOverABetterTitledOne is the case ObiWan's finding
// pointed at, made concrete. Searching "Antes de amanecer" really does return a
// 360p upload ranked above a 1080p one, and the 360p title scores better. The
// preferred-floor pass is what stops the library gaining another 450x360 file.
func TestAHigherResCandidateWinsOverABetterTitledOne(t *testing.T) {
	t.Parallel()
	// Only the second candidate has anything at 480p or above.
	tool := &floorAwareTool{ok: map[string]int{"low": 360, "high": 1080}}
	svc, _, movies := newTrailerService(t, tool)
	mkFilm(t, movies, "Antes de amanecer (1995)", "p.mkv")

	tool.results = []Candidate{
		{ID: "low", Title: "Antes de amanecer (1995) - Trailer oficial subtitulado", Duration: 95},
		{ID: "high", Title: "Tráiler Antes de amanecer", Duration: 122},
	}

	films, _ := svc.scan(t.Context())
	if _, err := svc.fetch(t.Context(), films, &jobs.Progress{}); err != nil {
		t.Fatal(err)
	}
	res := svc.Results()
	if len(res) != 1 || res[0].Err != "" {
		t.Fatalf("results = %+v", res)
	}
	if tool.took != "high" {
		t.Errorf("took %q, want the 1080p upload even though the other titled better", tool.took)
	}
}

// floorAwareTool answers like yt-dlp does: a download fails when the video has
// nothing at or above the requested floor.
type floorAwareTool struct {
	fakeTool
	ok   map[string]int // video id -> best height available
	took string
}

func (f *floorAwareTool) Download(ctx context.Context, url, dir, stem string, floor int) error {
	id := url[strings.LastIndex(url, "=")+1:]
	if f.ok[id] < floor {
		return ErrTooLowRes
	}
	f.took = id
	return f.fakeTool.Download(ctx, url, dir, stem, floor)
}
