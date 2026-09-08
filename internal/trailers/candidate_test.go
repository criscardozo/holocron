package trailers

import "testing"

// TestRejectsWhatIsNotATrailer is the test the script this replaces would fail.
// It took the first result, so any of these could end up in the library named
// "<film>-trailer.mp4" and nobody would know until they pressed play.
func TestRejectsWhatIsNotATrailer(t *testing.T) {
	t.Parallel()
	for _, c := range []Candidate{
		{ID: "a", Title: "THE MATRIX Trailer REACTION!!", Duration: 1200},
		{ID: "b", Title: "The Matrix ending explained", Duration: 700},
		{ID: "c", Title: "The Matrix (1999) FULL MOVIE", Duration: 8160},
		{ID: "d", Title: "The Matrix - Behind the Scenes", Duration: 400},
		{ID: "e", Title: "The Matrix soundtrack", Duration: 200},
		{ID: "f", Title: "The Matrix fan made trailer", Duration: 120},
		{ID: "g", Title: "The Matrix trailer", Duration: 9}, // a clip, not a trailer
	} {
		if n, _ := score(c, "The Matrix", 1999); n > 0 {
			t.Errorf("%q scored %d, should have been rejected", c.Title, n)
		}
	}
}

// TestPrefersTheOriginalOverTheDubbed is the whole point of the ordering here:
// original audio with Spanish subtitles, not a Spanish dub.
func TestPrefersTheOriginalOverTheDubbed(t *testing.T) {
	t.Parallel()
	cands := []Candidate{
		{ID: "dub", Title: "DUNE Trailer Español Latino DOBLADO", Duration: 150},
		{ID: "orig", Title: "DUNE - Official Trailer", Duration: 155},
		{ID: "subbed", Title: "DUNE Trailer Subtitulado en Español", Duration: 150},
	}
	best, reason, ok := Pick(cands, "Dune", 2021)
	if !ok {
		t.Fatal("nothing was picked")
	}
	if best.ID == "dub" {
		t.Errorf("picked the dubbed one: %q (%s)", best.Title, reason)
	}
}

// TestADubbedTrailerBeatsNoTrailer, because for the older Argentine and Mexican
// films in this library it is sometimes all there is.
func TestADubbedTrailerBeatsNoTrailer(t *testing.T) {
	t.Parallel()
	cands := []Candidate{
		{ID: "dub", Title: "Esperando la carroza - Trailer oficial español latino", Duration: 120},
	}
	if _, _, ok := Pick(cands, "Esperando la carroza", 1985); !ok {
		t.Error("refused the only trailer there is")
	}
}

// TestPicksTheRightFilm. A search for a common title returns other films, and
// the wrong one arrives looking perfectly normal.
func TestPicksTheRightFilm(t *testing.T) {
	t.Parallel()
	cands := []Candidate{
		{ID: "wrong", Title: "Heat 2 - Official Trailer", Duration: 140},
		{ID: "other", Title: "Body Heat - Official Trailer", Duration: 140},
		{ID: "right", Title: "Heat (1995) - Official Trailer", Duration: 145},
	}
	best, _, ok := Pick(cands, "Heat", 1995)
	if !ok || best.ID != "right" {
		t.Errorf("picked %q, want the 1995 one", best.Title)
	}
}

// TestRejectsADifferentFilmEntirely guards the case where the search finds
// nothing relevant at all — better to report no trailer than to file the wrong
// film's under this one's name.
func TestRejectsADifferentFilmEntirely(t *testing.T) {
	t.Parallel()
	cands := []Candidate{
		{ID: "x", Title: "Top Gun Maverick - Official Trailer", Duration: 150},
	}
	if best, _, ok := Pick(cands, "Como agua para chocolate", 1992); ok {
		t.Errorf("accepted %q for a completely different film", best.Title)
	}
}

// TestAccentsDoNotBreakTheMatch, since half this library is in Spanish and
// YouTube titles are inconsistent about them.
func TestAccentsDoNotBreakTheMatch(t *testing.T) {
	t.Parallel()
	cands := []Candidate{
		{ID: "y", Title: "EL SECRETO DE SUS OJOS - Trailer oficial", Duration: 130},
	}
	if _, _, ok := Pick(cands, "El secreto de sus ojos", 2009); !ok {
		t.Error("an accented title failed to match its own trailer")
	}
}

// TestReasonIsRecorded. A wrong trailer is silent, so the choice has to be
// explainable after the fact without re-running the search.
func TestReasonIsRecorded(t *testing.T) {
	t.Parallel()
	cands := []Candidate{{ID: "z", Title: "Arrival - Official Trailer", Duration: 150}}
	_, reason, ok := Pick(cands, "Arrival", 2016)
	if !ok || reason == "" {
		t.Fatalf("no reason recorded (ok=%v)", ok)
	}
}
