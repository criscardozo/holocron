package naming

import "testing"

// TestParsePicksTheReleaseYearNotTheFirstOne is the case that made this parser
// necessary. The old code took the first four digits that looked like a year,
// which gets every film with a number in its title wrong — and gets it wrong
// plausibly, which is how a bulk rename ends up scraping the wrong film.
func TestParsePicksTheReleaseYearNotTheFirstOne(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw   string
		title string
		year  int
	}{
		{"Blade.Runner.2049.2017", "Blade Runner 2049", 2017},
		{"2012.2009", "2012", 2009},
		{"1917.2019", "1917", 2019},
		{"Blade Runner 2049 (2017)", "Blade Runner 2049", 2017},
	}
	for _, c := range cases {
		title, year, ok := Parse(c.raw)
		if !ok || title != c.title || year != c.year {
			t.Errorf("Parse(%q) = %q, %d, %v; want %q, %d, true",
				c.raw, title, year, ok, c.title, c.year)
		}
	}
}

// TestParseStripsWhatDescribesTheFileNotTheFilm.
func TestParseStripsWhatDescribesTheFileNotTheFilm(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"The.Matrix.1999.1080p.BluRay":                   "The Matrix",
		"Mad.Max.Fury.Road.2015.2160p.WEB-DL.x265-GROUP": "Mad Max Fury Road",
		"Ocean's.Eleven.2001.1080p.BluRay.x264-AMIABLE":  "Ocean's Eleven",
		"Arrival 2016 REMUX HDR10 TrueHD Atmos":          "Arrival",
	}
	for raw, want := range cases {
		if title, _, _ := Parse(raw); title != want {
			t.Errorf("Parse(%q) title = %q, want %q", raw, title, want)
		}
	}
}

// TestParseKeepsProseIntact. A tag word is only junk at the end of a name, and
// full stops are only separators when the name is written in them.
func TestParseKeepsProseIntact(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Mr. Nobody (2009)":             "Mr. Nobody",
		"El secreto de sus ojos (2009)": "El secreto de sus ojos",
		// "Dual" is a release tag, but here it is the film.
		"Dual (2022)": "Dual",
		// Only the trailing tag goes; the one inside the title stays.
		"Cam (2018)": "Cam",
	}
	for raw, want := range cases {
		if title, _, _ := Parse(raw); title != want {
			t.Errorf("Parse(%q) title = %q, want %q", raw, title, want)
		}
	}
}

// TestParseRefusesToInventAYear. Every name here could be given a year by
// guessing, and guessing is what sends the scraper to a different film behind a
// name that looks deliberate.
func TestParseRefusesToInventAYear(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"Esperando la carroza",
		"Como agua para chocolate",
		"2012 1080p BluRay", // the only number present is the title
		"",
	} {
		if title, year, ok := Parse(raw); ok {
			t.Errorf("Parse(%q) invented %q (%d)", raw, title, year)
		}
	}
}

// TestEditionTagsSurviveAndGoAfterTheYear pins Jellyfin's own syntax: the tag
// only counts as an edition where Jellyfin looks for it.
func TestEditionTagsSurviveAndGoAfterTheYear(t *testing.T) {
	t.Parallel()
	const raw = "Dune Part Two 2024 {edition-IMAX}"
	title, year, ok := Parse(raw)
	if !ok || title != "Dune Part Two" || year != 2024 {
		t.Fatalf("Parse(%q) = %q, %d, %v", raw, title, year, ok)
	}
	if eds := Editions(raw); len(eds) != 1 || eds[0] != "{edition-IMAX}" {
		t.Fatalf("Editions(%q) = %v", raw, eds)
	}
	_, suggestion := Validate(raw)
	if suggestion != "Dune Part Two (2024) {edition-IMAX}" {
		t.Errorf("suggestion = %q", suggestion)
	}
	if ok, _ := Validate(suggestion); !ok {
		t.Errorf("the suggestion %q is not itself valid", suggestion)
	}
}

// TestEverySuggestionIsItselfValid closes the loop the rename depends on: if a
// suggested name would not pass validation, applying it leaves the folder
// flagged forever and the next pass renames it again.
func TestEverySuggestionIsItselfValid(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"The.Matrix.1999.1080p.BluRay",
		"Blade.Runner.2049.2017",
		"2012.2009",
		"Mad.Max.Fury.Road.2015.2160p.WEB-DL.x265-GROUP",
		"Dune Part Two 2024 {edition-IMAX}",
		"  Arrival 2016  ",
	} {
		ok, suggestion := Validate(raw)
		if ok {
			continue
		}
		if valid, _ := Validate(suggestion); !valid {
			t.Errorf("Validate(%q) suggested %q, which is not valid", raw, suggestion)
		}
	}
}
