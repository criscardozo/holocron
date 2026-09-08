package trailers

import (
	"context"
	"fmt"
	"strings"
)

// The order the queries are tried in is the preference, spelled out. The script
// this replaces searched "trailer subtitulado", then "trailer español", then
// "trailer" — which puts a Spanish dub ahead of the original, the opposite of
// what is wanted here. Original audio first, Spanish subtitles second, anything
// third.
//
// They are tried in order and the first good enough answer wins, rather than
// all three being run and the best taken. Each query is a yt-dlp invocation
// against the network, and at 181 films the difference is minutes against an
// hour. Falling through only happens when the first query found nothing worth
// having, which is exactly the case that deserves the extra look.

// goodEnough is the score above which the search stops looking. An official
// trailer of the right length whose title matches clears it comfortably;
// anything scraped together from a vague match does not.
const goodEnough = 9

// Queries returns the searches to try, in order of preference.
func Queries(title string, year int) []string {
	y := ""
	if year > 0 {
		y = " " + itoa(year)
	}
	return []string{
		`"` + title + `"` + y + " official trailer",
		title + y + " trailer subtitulado español",
		title + y + " trailer",
	}
}

// searchResults is how many results to ask for per query. Enough that the right
// one is in there when it is not ranked first, few enough to stay quick.
const searchResults = 8

// Searcher is what Find needs, so a test can answer without a network.
type Searcher interface {
	Search(ctx context.Context, query string, n int) ([]Candidate, error)
}

// Found is a chosen trailer and why it was chosen.
type Found struct {
	Candidate Candidate
	Reason    string
	Query     string
}

// ErrNotFound means no query turned up anything that is plausibly this film's
// trailer. Reported rather than papered over with the best of a bad set: a
// wrong trailer is worse than none, because nobody re-checks a file that is
// already there.
var ErrNotFound = fmt.Errorf("no trailer found")

// Find searches for a film's trailer.
func Find(ctx context.Context, s Searcher, title string, year int) (Found, error) {
	var best Found
	bestScore := 0

	for _, q := range Queries(title, year) {
		if err := ctx.Err(); err != nil {
			return Found{}, err
		}
		cands, err := s.Search(ctx, q, searchResults)
		if err != nil {
			// A broken extractor will not fix itself on the next query, so
			// stop rather than making two more doomed network calls per film.
			return Found{}, err
		}
		for _, c := range cands {
			n, why := score(c, title, year)
			if n > bestScore {
				best, bestScore = Found{Candidate: c, Reason: why, Query: q}, n
			}
		}
		if bestScore >= goodEnough {
			return best, nil
		}
	}
	if bestScore <= 0 {
		return Found{}, ErrNotFound
	}
	return best, nil
}

// Stem is the filename a trailer should take for a folder: the folder's own
// name. Keeps the library consistent with itself instead of with YouTube.
func Stem(folder string) string { return strings.TrimSpace(folder) }
