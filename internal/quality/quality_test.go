package quality

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/jellyfin"
)

// testNow is the instant every test judges "has this aired" against.
var testNow = time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)

func num(n int) *int { return &n }

// episode builds an on-disk episode with the given streams.
func episode(id, series, name string, season, ep int, streams ...jellyfin.MediaStream) jellyfin.Item {
	return jellyfin.Item{
		ID: id, Type: jellyfin.TypeEpisode, Name: name,
		Path:       "/media/series/" + series + "/" + name + ".mkv",
		SeriesName: series, SeriesID: "series-" + series,
		Season: num(season), Episode: num(ep),
		Overview: "Pasan cosas.",
		Streams:  streams,
	}
}

func subs(lang string) jellyfin.MediaStream {
	return jellyfin.MediaStream{Type: "Subtitle", Language: lang}
}

func audio(lang string) jellyfin.MediaStream {
	return jellyfin.MediaStream{Type: "Audio", Language: lang}
}

func countOf(t *testing.T, r Report, c Category) int {
	t.Helper()
	return r.Count(c)
}

func TestSubtitleFindingsSkipSeries(t *testing.T) {
	t.Parallel()
	// A series is a directory with no streams of its own. Counting it as
	// missing subtitles would add one false finding per show in the library.
	items := []jellyfin.Item{
		{ID: "s1", Type: jellyfin.TypeSeries, Name: "The Bear",
			Path: "/media/series/The Bear", Overview: "Un cocinero."},
		episode("e1", "The Bear", "System", 1, 1, audio("eng"), subs("spa")),
		episode("e2", "The Bear", "Hands", 1, 2, audio("eng")),
		episode("e3", "The Bear", "Brigade", 1, 3, audio("eng"), subs("")),
		episode("e4", "The Bear", "Sheridan", 1, 4, audio("spa")),
	}
	r := Analyse(items, testNow)

	if got := countOf(t, r, CatSubsMissing); got != 2 {
		t.Fatalf("subs-missing = %d, want 2 (the series must not be counted)", got)
	}
	found := r.For(CatSubsMissing)
	var details []string
	for _, f := range found {
		details = append(details, f.Detail)
		if f.Kind != "episodio" {
			t.Errorf("finding on a %s: %+v", f.Kind, f)
		}
	}
	// The unlabelled subtitle is reported apart from the absent one: next to an
	// Argentine film it is probably Spanish, next to an anime probably not.
	if !slices.ContainsFunc(details, func(d string) bool {
		return strings.Contains(d, "sin idioma")
	}) {
		t.Errorf("expected the unlabelled subtitle to be called out, got %v", details)
	}
	// Spanish audio needs no subtitles at all.
	for _, f := range found {
		if strings.Contains(f.Title, "Sheridan") {
			t.Error("an item with Spanish audio is not missing subtitles")
		}
	}
}

func TestGhostsAreReportedOnce(t *testing.T) {
	t.Parallel()
	// No file, no synopsis, no title: it is still one finding, because the fix
	// is the same for all three and it is not "refresh the metadata".
	r := Analyse([]jellyfin.Item{
		{ID: "g1", Type: jellyfin.TypeEpisode, Name: "", Path: "",
			SeriesName: "Lost", Season: num(2), Episode: num(5)},
	}, testNow)

	if got := r.Count(CatGhost); got != 1 {
		t.Fatalf("ghosts = %d, want 1", got)
	}
	for _, c := range []Category{CatNoSynopsis, CatGenericTitle, CatSubsMissing} {
		if got := r.Count(c); got != 0 {
			t.Errorf("%s = %d, want 0: a ghost is only a ghost", c, got)
		}
	}
	if got := r.For(CatGhost)[0].Title; !strings.Contains(got, "S02E05") {
		t.Errorf("title = %q, want the episode code in it", got)
	}
}

func TestNumberingCollision(t *testing.T) {
	t.Parallel()
	items := []jellyfin.Item{
		episode("a", "Chernobyl", "1:23:45", 1, 1, audio("spa")),
		// Same number, different file: one of the two is unreachable.
		episode("b", "Chernobyl", "Please Remain Calm", 1, 1, audio("spa")),
		episode("c", "Chernobyl", "Open Wide, O Earth", 1, 3, audio("spa")),
		// Same number but a different series: not a collision.
		episode("d", "Fargo", "The Crocodile's Dilemma", 1, 1, audio("spa")),
	}
	r := Analyse(items, testNow)

	if got := r.Count(CatCollision); got != 2 {
		t.Fatalf("collisions = %d, want 2 (both sides of the clash)", got)
	}
	for _, f := range r.For(CatCollision) {
		if !strings.Contains(f.Detail, "S01E01") || !strings.Contains(f.Detail, "2 archivos") {
			t.Errorf("detail = %q, want the code and how many files share it", f.Detail)
		}
	}
}

func TestUnnumberedEpisodeIsNotACollision(t *testing.T) {
	t.Parallel()
	// Two episodes Jellyfin could not place. They share "no number", which is
	// not the same as sharing a number — pairing them would be a false finding.
	a := episode("a", "Docus", "Parte uno", 0, 0, audio("spa"))
	b := episode("b", "Docus", "Parte dos", 0, 0, audio("spa"))
	a.Season, a.Episode = nil, nil
	b.Season, b.Episode = nil, nil

	if got := Analyse([]jellyfin.Item{a, b}, testNow).Count(CatCollision); got != 0 {
		t.Errorf("collisions = %d, want 0", got)
	}
}

func TestGenericTitles(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		generic bool
	}{
		{"Episode 4", true},
		{"Episodio 12", true},
		{"Capítulo 3", true},
		{"Untitled", true},
		{"", true},
		{"The.Bear.S01E01.1080p.WEB-DL.x264", true},
		{"Dune.Parte.Dos.2024", true},
		// Real titles that must not trip the rules.
		{"Episodes", false},
		{"1080 Degrees", false},
		{"El secreto de sus ojos", false},
		{"Capítulo final: la fuga", false},
		{"9", false},
	}
	for _, tc := range cases {
		it := jellyfin.Item{ID: "x", Type: jellyfin.TypeMovie, Name: tc.name,
			Path: "/media/peliculas/x.mkv", Overview: "algo", Streams: []jellyfin.MediaStream{audio("spa")}}
		got := Analyse([]jellyfin.Item{it}, testNow).Count(CatGenericTitle)
		if (got == 1) != tc.generic {
			t.Errorf("%q: generic = %v, want %v", tc.name, got == 1, tc.generic)
		}
	}
}

func TestMissingSynopsis(t *testing.T) {
	t.Parallel()
	blank := episode("a", "Show", "Uno", 1, 1, audio("spa"))
	blank.Overview = "   \n"
	ok := episode("b", "Show", "Dos", 1, 2, audio("spa"))

	r := Analyse([]jellyfin.Item{blank, ok}, testNow)
	if got := r.Count(CatNoSynopsis); got != 1 {
		t.Fatalf("no-synopsis = %d, want 1 (whitespace is not a synopsis)", got)
	}
}

func TestTruncationKeepsTheRealCount(t *testing.T) {
	t.Parallel()
	// More findings than the display cap: the counter must stay honest, or the
	// panel reads as "200 problems" when there are far more.
	var items []jellyfin.Item
	for i := range maxPerCategory + 37 {
		it := episode("e", "Show", "Ep", 1, i, audio("eng"))
		it.ID = "id-" + episodeCode(1, i)
		items = append(items, it)
	}
	r := Analyse(items, testNow)

	if got := r.Count(CatSubsMissing); got != maxPerCategory+37 {
		t.Errorf("count = %d, want %d", got, maxPerCategory+37)
	}
	if got := len(r.For(CatSubsMissing)); got != maxPerCategory {
		t.Errorf("shown = %d, want the cap %d", got, maxPerCategory)
	}
	if !r.Truncated(CatSubsMissing) {
		t.Error("the category must report itself as truncated")
	}
}

func TestReportIsDeterministic(t *testing.T) {
	t.Parallel()
	// Two audits of an unchanged library must produce the same report. Findings
	// gathered from maps (collisions) would otherwise land in a random order,
	// and the display cap would keep a different subset each run.
	items := []jellyfin.Item{
		episode("a", "Zeta", "Uno", 1, 1, audio("eng")),
		episode("b", "Alfa", "Dos", 2, 2, audio("eng")),
		episode("c", "Alfa", "Tres", 2, 2, audio("eng")),
		{ID: "g", Type: jellyfin.TypeMovie, Name: "Fantasma", Path: ""},
	}
	first := Analyse(items, testNow)
	for range 20 {
		got := Analyse(items, testNow)
		if len(got.Findings) != len(first.Findings) {
			t.Fatalf("finding count moved: %d vs %d", len(got.Findings), len(first.Findings))
		}
		for i := range got.Findings {
			if got.Findings[i] != first.Findings[i] {
				t.Fatalf("finding %d differs:\n %+v\n %+v", i, got.Findings[i], first.Findings[i])
			}
		}
	}
	// Categories come out in display order, so the page renders top to bottom
	// without sorting again.
	order := map[Category]int{}
	for i, c := range Categories {
		order[c] = i
	}
	for i := 1; i < len(first.Findings); i++ {
		if order[first.Findings[i-1].Category] > order[first.Findings[i].Category] {
			t.Fatalf("findings are not grouped in display order at %d", i)
		}
	}
}

func TestMentionsGuardsTheRefreshTarget(t *testing.T) {
	t.Parallel()
	r := Analyse([]jellyfin.Item{
		{ID: "real", Type: jellyfin.TypeMovie, Name: "Sin sinopsis",
			Path: "/media/peliculas/x.mkv", Streams: []jellyfin.MediaStream{audio("spa")}},
	}, testNow)
	if !r.Mentions("real") {
		t.Error("an item that produced a finding must be refreshable")
	}
	if r.Mentions("../../etc/passwd") {
		t.Error("an id the report never saw must be rejected")
	}
}

// TestAnUnairedEpisodeIsNotAGhost is the near miss this category exists for.
//
// Jellyfin lists an in-progress season's future episodes with no file, which
// looks exactly like a file that was deleted. The panel called both "ghosts"
// and told the user to clean them up — which for the first kind meant deleting
// the season they were in the middle of watching. Measured on the real library:
// six of one show's eleven ghosts were unaired and the first came out the next
// day.
func TestAnUnairedEpisodeIsNotAGhost(t *testing.T) {
	t.Parallel()
	future := jellyfin.Item{
		ID: "a", Type: jellyfin.TypeEpisode, Name: "Everything Beautiful",
		SeriesName: "Materia oscura", Premiere: testNow.Add(24 * time.Hour),
	}
	deleted := jellyfin.Item{
		ID: "b", Type: jellyfin.TypeEpisode, Name: "Episodio viejo",
		SeriesName: "En el barro", Premiere: testNow.Add(-365 * 24 * time.Hour),
	}

	r := Analyse([]jellyfin.Item{future, deleted}, testNow)

	if got := r.Count(CatGhost); got != 1 {
		t.Errorf("ghosts = %d, want only the one that actually aired", got)
	}
	if got := r.Count(CatUnaired); got != 1 {
		t.Errorf("unaired = %d, want the future episode", got)
	}
	// And the two must never be counted together again.
	for _, f := range r.For(CatGhost) {
		if strings.Contains(f.Title, "Everything Beautiful") {
			t.Error("an unaired episode is being reported as a ghost")
		}
	}
}

// TestAnUnknownAirDateIsTreatedAsAired. Plenty of older material has no
// premiere date, and reading "unknown" as "in the future" would hide a real
// missing file behind a reassuring label — the same mistake in the other
// direction.
func TestAnUnknownAirDateIsTreatedAsAired(t *testing.T) {
	t.Parallel()
	noDate := jellyfin.Item{
		ID: "c", Type: jellyfin.TypeEpisode, Name: "Sin fecha",
		SeriesName: "Algo habrán hecho",
	}
	r := Analyse([]jellyfin.Item{noDate}, testNow)
	if got := r.Count(CatGhost); got != 1 {
		t.Errorf("ghosts = %d, want the undated one counted as missing", got)
	}
	if got := r.Count(CatUnaired); got != 0 {
		t.Errorf("unaired = %d, want none", got)
	}
}

// TestTheGhostAdviceDoesNotTellYouToDeleteASeasonInProgress. The hint is the
// part a person acts on, so it is the part that has to be right.
func TestTheGhostAdviceDoesNotTellYouToDeleteASeasonInProgress(t *testing.T) {
	t.Parallel()
	if hint := CatUnaired.Hint(); !strings.Contains(hint, "No los borres") {
		t.Errorf("the unaired hint should say not to delete them, got %q", hint)
	}
	if hint := CatGhost.Hint(); !strings.Contains(hint, "Ya salieron") {
		t.Errorf("the ghost hint should say it only covers what has aired, got %q", hint)
	}
}
