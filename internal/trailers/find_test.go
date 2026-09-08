package trailers

import (
	"context"
	"errors"
	"testing"
)

// fakeSearch answers per query, so the order of preference can be asserted
// without a network.
type fakeSearch struct {
	byQuery map[string][]Candidate
	calls   []string
	err     error
}

func (f *fakeSearch) Search(_ context.Context, q string, _ int) ([]Candidate, error) {
	f.calls = append(f.calls, q)
	if f.err != nil {
		return nil, f.err
	}
	return f.byQuery[q], nil
}

// TestTheOriginalIsSearchedFirst pins the ordering. The script this replaces
// searched Spanish first, which is the opposite of the preference here.
func TestTheOriginalIsSearchedFirst(t *testing.T) {
	t.Parallel()
	qs := Queries("Dune", 2021)
	if len(qs) != 3 {
		t.Fatalf("queries = %v", qs)
	}
	if !contains(qs[0], "official trailer") {
		t.Errorf("the first query should look for the original: %q", qs[0])
	}
	if !contains(qs[1], "subtitulado") {
		t.Errorf("the second should look for subtitles: %q", qs[1])
	}
}

// TestAGoodFirstAnswerStopsTheSearch. Each query is a network call, and at 181
// films the difference between one and three is minutes against an hour.
func TestAGoodFirstAnswerStopsTheSearch(t *testing.T) {
	t.Parallel()
	qs := Queries("Dune", 2021)
	f := &fakeSearch{byQuery: map[string][]Candidate{
		qs[0]: {{ID: "a", Title: "Dune (2021) - Official Trailer", Duration: 150}},
	}}
	got, err := Find(t.Context(), f, "Dune", 2021)
	if err != nil {
		t.Fatal(err)
	}
	if got.Candidate.ID != "a" {
		t.Errorf("picked %q", got.Candidate.ID)
	}
	if len(f.calls) != 1 {
		t.Errorf("made %d searches, want 1: %v", len(f.calls), f.calls)
	}
}

// TestItFallsThroughWhenTheFirstQueryIsWeak, which is the case that deserves
// the extra look.
func TestItFallsThroughWhenTheFirstQueryIsWeak(t *testing.T) {
	t.Parallel()
	qs := Queries("Esperando la carroza", 1985)
	f := &fakeSearch{byQuery: map[string][]Candidate{
		qs[0]: {{ID: "junk", Title: "Reacción a Esperando la carroza", Duration: 900}},
		qs[2]: {{ID: "real", Title: "Esperando la carroza - Trailer", Duration: 120}},
	}}
	got, err := Find(t.Context(), f, "Esperando la carroza", 1985)
	if err != nil {
		t.Fatal(err)
	}
	if got.Candidate.ID != "real" {
		t.Errorf("picked %q, want the real trailer", got.Candidate.ID)
	}
	if len(f.calls) != 3 {
		t.Errorf("should have tried every query, made %d", len(f.calls))
	}
}

// TestNoTrailerIsReportedAsSuch rather than settling for the best of a bad set.
// A wrong trailer is worse than none: nobody re-checks a file that is there.
func TestNoTrailerIsReportedAsSuch(t *testing.T) {
	t.Parallel()
	qs := Queries("Cien veces no debo", 1990)
	f := &fakeSearch{byQuery: map[string][]Candidate{
		qs[0]: {{ID: "x", Title: "Top Gun - Official Trailer", Duration: 150}},
	}}
	if _, err := Find(t.Context(), f, "Cien veces no debo", 1990); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// TestABrokenExtractorStopsImmediately instead of making two more doomed calls
// per film, which at 181 films is the difference between noticing and waiting.
func TestABrokenExtractorStopsImmediately(t *testing.T) {
	t.Parallel()
	f := &fakeSearch{err: ErrExtractorBroken}
	if _, err := Find(t.Context(), f, "Dune", 2021); !errors.Is(err, ErrExtractorBroken) {
		t.Errorf("err = %v", err)
	}
	if len(f.calls) != 1 {
		t.Errorf("kept trying after the extractor failed: %v", f.calls)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
