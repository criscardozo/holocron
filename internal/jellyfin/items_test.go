package jellyfin

import "testing"

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
