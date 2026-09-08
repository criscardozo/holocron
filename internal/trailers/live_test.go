package trailers

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// TestAgainstRealYouTube runs the whole search against YouTube for real. Opt-in
// through HOLOCRON_LIVE_YOUTUBE=1, because it makes network calls and depends
// on a tool that is deliberately not a build dependency — it must never run in
// CI, and it must be possible to run on purpose.
//
// It exists because the unit tests can only prove the scoring behaves on
// fixtures I wrote, and I am the same person who decided what a good candidate
// looks like. This is the half that checks the fixtures resemble YouTube.
//
// Last run picked the official trailer for all six, including the four older
// Argentine and Mexican films that were expected to be the hard ones:
//
//	The Matrix                140s  The Matrix (1999) Official Trailer #1
//	Dune                      185s  Dune Official Trailer
//	Esperando la carroza      132s  Esperando la carroza (1985) [Trailer oficial]
//	Como agua para chocolate  168s  Como Agua Para Chocolate | Trailer Oficial | Max
//	El secreto de sus ojos    174s  EL SECRETO DE SUS OJOS (2009) | TRÁILER OFICIAL
//	Homo Argentum              65s  HOMO ARGENTUM - Tráiler Oficial
func TestAgainstRealYouTube(t *testing.T) {
	if os.Getenv("HOLOCRON_LIVE_YOUTUBE") != "1" {
		t.Skip("set HOLOCRON_LIVE_YOUTUBE=1 to run this against the network")
	}
	r := NewRunner()
	if !r.Available() {
		t.Skip("yt-dlp is not installed here")
	}
	r.timeout = 90 * time.Second

	films := []struct {
		title string
		year  int
	}{
		{"The Matrix", 1999},
		{"Dune", 2021},
		{"Esperando la carroza", 1985},
		{"Como agua para chocolate", 1992},
		{"El secreto de sus ojos", 2009},
		{"Homo Argentum", 2025},
	}
	for _, f := range films {
		got, err := Find(t.Context(), r, f.title, f.year)
		if err != nil {
			t.Errorf("%s: %v", f.title, err)
			continue
		}
		best := got[0]
		fmt.Printf("%-30s %4ds  %-55.55s  [%s]  (+%d alternativas)\n",
			f.title, best.Candidate.Duration, best.Candidate.Title, best.Reason, len(got)-1)
		if !saysTrailer.MatchString(best.Candidate.Title) {
			t.Errorf("%s: picked something that does not say trailer: %q", f.title, best.Candidate.Title)
		}
	}
}
