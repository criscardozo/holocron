package trailers

import (
	"context"
	"fmt"
	"sort"
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

// maxAttempts bounds how many candidates the caller may try. More than a
// couple means the search did not really find the trailer, and each attempt is
// a download that may run for a while.
const maxAttempts = 3

// Find searches for a film's trailer and returns the plausible candidates, best
// first.
//
// A list rather than one answer, because the search cannot see everything that
// disqualifies a video. Resolution is the case that forced it: a flat search
// does not report it, so the floor is applied when downloading instead, and a
// candidate rejected there needs a next one. The same goes for a video YouTube
// has since removed.
//
// The reason is that structure, not speed. Asking the search for resolution is
// slower — 6 s a query against 14 s on the Pi, measured there — but 2.3x is a
// difference this could afford, and an earlier version of this comment claimed
// 15x from a measurement taken on a laptop with fibre. It does not transfer.
// What does not change with hardware is that the download can still refuse a
// candidate the search approved, so the caller needs somewhere to go next
// either way.
func Find(ctx context.Context, s Searcher, title string, year int) ([]Found, error) {
	type scored struct {
		f Found
		n int
	}
	var all []scored
	seen := make(map[string]bool)
	best := 0

	for _, q := range Queries(title, year) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cands, err := s.Search(ctx, q, searchResults)
		if err != nil {
			// A broken extractor will not fix itself on the next query, so
			// stop rather than making two more doomed network calls per film.
			return nil, err
		}
		for _, c := range cands {
			if seen[c.ID] {
				continue
			}
			n, why := score(c, title, year)
			if n <= 0 {
				continue
			}
			seen[c.ID] = true
			all = append(all, scored{Found{Candidate: c, Reason: why, Query: q}, n})
			if n > best {
				best = n
			}
		}
		if best >= goodEnough {
			break
		}
	}
	if len(all) == 0 {
		return nil, ErrNotFound
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].n > all[j].n })

	out := make([]Found, 0, len(all))
	for _, sc := range all {
		out = append(out, sc.f)
	}
	if len(out) > maxAttempts {
		out = out[:maxAttempts]
	}
	return out, nil
}

// Stem is the filename a trailer should take for a folder: the folder's own
// name. Keeps the library consistent with itself instead of with YouTube.
func Stem(folder string) string { return strings.TrimSpace(folder) }
