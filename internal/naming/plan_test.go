package naming

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// mkLibrary builds a throwaway media root and returns an os.Root over it.
func mkLibrary(t *testing.T, folder string, files ...string) *os.Root {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func names(t *testing.T, root *os.Root, dir string) []string {
	t.Helper()
	entries, err := readDirIn(root, dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// TestSubtitlesFollowTheVideo is the point of the whole feature. A subtitle is
// matched to its video by filename, so renaming one without the other silently
// costs the subtitles — and nothing warns you, the film just plays without them.
func TestSubtitlesFollowTheVideo(t *testing.T) {
	t.Parallel()
	const folder = "The.Matrix.1999.1080p.BluRay"
	root := mkLibrary(t, folder,
		"The.Matrix.1999.1080p.BluRay.mkv",
		"The.Matrix.1999.1080p.BluRay.es.srt",
		"The.Matrix.1999.1080p.BluRay.en.srt",
		"The.Matrix.1999.1080p.BluRay-poster.jpg",
		"poster.jpg",
	)

	p, err := PlanFolder(root, folder)
	if err != nil {
		t.Fatal(err)
	}
	if p.NewFolder != "The Matrix (1999)" {
		t.Fatalf("NewFolder = %q", p.NewFolder)
	}
	if _, err := Apply(root, p); err != nil {
		t.Fatal(err)
	}

	got := names(t, root, "The Matrix (1999)")
	want := []string{
		"The Matrix (1999)-poster.jpg",
		"The Matrix (1999).en.srt",
		"The Matrix (1999).es.srt",
		"The Matrix (1999).mkv",
		"poster.jpg", // already the name Jellyfin looks for
	}
	sort.Strings(want)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

// TestAFolderWithoutAYearIsLeftAlone. Inventing a year would send the scraper
// to a different film and the name would look deliberate, which is worse than
// an obviously wrong one.
func TestAFolderWithoutAYearIsLeftAlone(t *testing.T) {
	t.Parallel()
	root := mkLibrary(t, "Esperando la carroza", "Esperando la carroza.mkv")

	p, err := PlanFolder(root, "Esperando la carroza")
	if err != nil {
		t.Fatal(err)
	}
	if p.Blocked == "" {
		t.Fatal("a folder with no year must be blocked, not guessed at")
	}
	if len(p.Files) != 0 || p.NewFolder != p.Folder {
		t.Errorf("a blocked plan must be empty, got %+v", p)
	}
	if _, err := Apply(root, p); err == nil {
		t.Error("applying a blocked plan must fail rather than do half of it")
	}
	if got := names(t, root, "Esperando la carroza"); len(got) != 1 {
		t.Errorf("the folder was touched: %v", got)
	}
}

// TestNothingIsOverwritten. rename(2) replaces the destination without a word,
// so in a library a careless rename does not create a mess, it deletes a film.
func TestNothingIsOverwritten(t *testing.T) {
	t.Parallel()
	const folder = "Heat.1995"
	root := mkLibrary(t, folder,
		"Heat.1995.mkv",
		"Heat (1995).mkv", // already correctly named, different file
	)

	p, err := PlanFolder(root, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 0 {
		t.Errorf("planned a rename onto an existing file: %+v", p.Files)
	}
	if len(p.Skipped) != 1 {
		t.Fatalf("the collision must be reported, got %+v", p.Skipped)
	}
	if _, err := Apply(root, p); err != nil {
		t.Fatal(err)
	}
	if got := names(t, root, "Heat (1995)"); len(got) != 2 {
		t.Errorf("a file disappeared: %v", got)
	}
}

// TestFilesThatDoNotShareTheNameAreLeftAlone guards the other direction: a
// rename pass that grabs everything in the folder would rewrite artwork whose
// names are already the ones Jellyfin looks for.
func TestFilesThatDoNotShareTheNameAreLeftAlone(t *testing.T) {
	t.Parallel()
	const folder = "Dune.2021"
	root := mkLibrary(t, folder,
		"Dune.2021.mkv", "poster.jpg", "fanart.jpg", "logo.png", "movie.nfo",
	)

	p, err := PlanFolder(root, folder)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range p.Files {
		if r.From != "Dune.2021.mkv" {
			t.Errorf("touched a file it should not have: %q", r.From)
		}
	}
}

// TestTheVideoStemWinsOverTheFolderName covers the folder whose contents
// disagree with it, which is most of a library assembled over years.
func TestTheVideoStemWinsOverTheFolderName(t *testing.T) {
	t.Parallel()
	const folder = "Blade.Runner.2049.2017"
	root := mkLibrary(t, folder,
		"BR2049.1080p.mkv",
		"BR2049.1080p.es.srt",
	)

	p, err := PlanFolder(root, folder)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root, p); err != nil {
		t.Fatal(err)
	}
	got := names(t, root, "Blade Runner 2049 (2017)")
	want := []string{"Blade Runner 2049 (2017).es.srt", "Blade Runner 2049 (2017).mkv"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestPlanningIsIdempotent. The screen invites a second press, and a library is
// scanned repeatedly, so running twice must be a no-op rather than a slow drift.
func TestPlanningIsIdempotent(t *testing.T) {
	t.Parallel()
	const folder = "Arrival.2016"
	root := mkLibrary(t, folder, "Arrival.2016.mkv", "Arrival.2016.es.srt")

	p, err := PlanFolder(root, folder)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root, p); err != nil {
		t.Fatal(err)
	}
	again, err := PlanFolder(root, "Arrival (2016)")
	if err != nil {
		t.Fatal(err)
	}
	if !again.Empty() {
		t.Errorf("a second pass would change things: %+v", again)
	}
}

// TestAStemMatchStopsAtASeparator. Without this "Alien 2" would match inside
// "Alien 2049" and produce a name built from the wrong film.
func TestAStemMatchStopsAtASeparator(t *testing.T) {
	t.Parallel()
	stems := []string{"Alien 2"}
	if got := matchStem("Alien 2049.mkv", stems); got != "" {
		t.Errorf("matched %q inside a longer name", got)
	}
	if got := matchStem("Alien 2.mkv", stems); got != "Alien 2" {
		t.Errorf("did not match the real stem, got %q", got)
	}
}
