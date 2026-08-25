package jellyfin

import (
	"encoding/json"
	"os"
	"testing"
)

// These decode responses captured from the real Jellyfin server this HTPC runs
// (see testdata/README.md). They exist to catch the ways the API surprised us:
// paths that are absent, subtitles that live on only one of several files, and
// provider keys that cannot be modelled as a struct.
func fixtures(t *testing.T) map[string]Item {
	t.Helper()
	raw, err := os.ReadFile("testdata/jellyfin-fixtures.json")
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var byCase map[string]Item
	if err := json.Unmarshal(raw, &byCase); err != nil {
		t.Fatalf("decode fixtures: %v", err)
	}
	return byCase
}

func fixture(t *testing.T, name string) Item {
	t.Helper()
	it, ok := fixtures(t)[name]
	if !ok {
		t.Fatalf("fixture %q missing", name)
	}
	return it
}

func TestExternalSubtitleCarriesItsPath(t *testing.T) {
	t.Parallel()
	it := fixture(t, "pelicula_con_subtitulo_externo")

	if it.Type != TypeMovie || it.Path == "" {
		t.Fatalf("unexpected item: %+v", it)
	}
	if !it.HasSpanishSubtitles() {
		t.Error("this title has a Spanish .srt beside it")
	}
	// Knowing the subtitle's path is what makes walking the directory
	// unnecessary.
	paths := it.SubtitlePaths()
	if len(paths) == 0 {
		t.Fatal("expected the external subtitle's path")
	}

	// Provider ids include keys with spaces, so they cannot be a struct.
	if _, ok := it.ProviderIDs["Imdb"]; !ok {
		t.Error("expected an Imdb provider id")
	}
	if _, ok := it.ProviderIDs["official website"]; !ok {
		t.Error("expected the key with a space to survive decoding")
	}
}

// The real "El mago de Oz": two versions, and only the 1080p one carries the
// Spanish subtitles. Looking at item.Path alone would report it as missing.
func TestSubtitlesOnASecondVersionStillCount(t *testing.T) {
	t.Parallel()
	it := fixture(t, "pelicula_multi_version")

	if len(it.Sources) < 2 {
		t.Fatalf("expected a multi-version item, got %d sources", len(it.Sources))
	}
	if got := len(it.FilePaths()); got != len(it.Sources) {
		t.Errorf("FilePaths returned %d paths for %d files", got, len(it.Sources))
	}
	if !it.HasSpanishSubtitles() {
		t.Error("one of the versions has Spanish subtitles, so the title is covered")
	}
	if got := it.SpanishSubtitleState(); got != SubsPresent {
		t.Errorf("state = %v, want SubsPresent", got)
	}
}

func TestSeriesPointAtTheirDirectory(t *testing.T) {
	t.Parallel()
	it := fixture(t, "serie")

	if it.Type != TypeSeries {
		t.Fatalf("type = %q, want Series", it.Type)
	}
	// A series has no file of its own, so its path already is the folder.
	if it.Folder() != it.Path {
		t.Errorf("folder = %q, want the item path %q", it.Folder(), it.Path)
	}
	if len(it.Sources) != 0 {
		t.Errorf("a series should carry no media sources, got %d", len(it.Sources))
	}
	// Anime carries provider ids beyond the usual three.
	if len(it.ProviderIDs) < 4 {
		t.Errorf("expected several provider ids, got %v", it.ProviderIDs)
	}
}

// Jellyfin lists episodes it knows from metadata providers but has never seen
// on disk. Acting on those means chasing files that do not exist.
func TestGhostEpisodeIsNotOnDisk(t *testing.T) {
	t.Parallel()
	it := fixture(t, "episodio_fantasma_sin_archivo")

	if it.OnDisk() {
		t.Error("an episode with no path is not on disk")
	}
	// It still comes with a media source, just an empty one — so the path list
	// has to stay empty rather than contain a blank entry.
	if got := it.FilePaths(); len(got) != 0 {
		t.Errorf("FilePaths = %v, want none", got)
	}
	if it.Folder() != "" {
		t.Errorf("folder = %q, want empty", it.Folder())
	}
}

func TestEpisodeWithAFileIsUsable(t *testing.T) {
	t.Parallel()
	it := fixture(t, "episodio_con_archivo")

	if !it.OnDisk() {
		t.Fatal("this episode has a file")
	}
	if it.Folder() == "" {
		t.Error("expected a folder for an episode on disk")
	}
}

func TestEveryFixtureDecodes(t *testing.T) {
	t.Parallel()
	all := fixtures(t)

	// The dump is curated to cover the cases that bit us; if one disappears,
	// the test above that relies on it would fail with a confusing message.
	for _, name := range []string{
		"pelicula_con_subtitulo_externo",
		"pelicula_con_subtitulos_internos",
		"pelicula_multi_version",
		"serie",
		"episodio_con_archivo",
		"episodio_fantasma_sin_archivo",
	} {
		it, ok := all[name]
		if !ok {
			t.Errorf("fixture %q missing", name)
			continue
		}
		if it.ID == "" || it.Name == "" {
			t.Errorf("fixture %q decoded empty: %+v", name, it)
		}
	}
}
