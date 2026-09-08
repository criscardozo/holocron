// Package trailers finds and downloads a trailer for a film that has none.
//
// The part that earns its keep is choosing which video to take. A YouTube
// search for "<film> trailer" returns the trailer, and also reaction videos,
// twenty-minute breakdowns, fan edits, clips, and sometimes the whole film.
// Taking the first result — which is what the script this replaces did — puts
// whatever YouTube felt like ranking first into the library under a name that
// says "trailer", and nobody finds out until they press play.
package trailers

import (
	"regexp"
	"strings"
	"unicode"
)

// Candidate is one search result, as much of it as yt-dlp reports.
type Candidate struct {
	ID       string
	Title    string
	Channel  string
	Duration int // seconds
}

// URL is where the video lives.
func (c Candidate) URL() string { return "https://www.youtube.com/watch?v=" + c.ID }

// A trailer is between about half a minute and five minutes. Outside that it is
// something else wearing the word: a teaser cut to nothing, or a review.
const (
	minDuration = 20
	maxDuration = 600
	// The range an actual theatrical trailer falls in.
	idealMin = 55
	idealMax = 220
)

var (
	// Words that mean the video is about the film rather than being its trailer.
	notATrailer = regexp.MustCompile(`(?i)\b(reaction|reacci[oó]n|review|rese[nñ]a|breakdown|explained|explicad[oa]|analysis|an[aá]lisis|recap|resumen|ending|final explicado|easter eggs|behind the scenes|making of|detr[aá]s de c[aá]maras|interview|entrevista|first time watching|fan.?made|fan.?trailer|concept|parody|parodia|edit|amv|full movie|pel[ií]cula completa|soundtrack|banda sonora|ost|score)\b`)
	// It says it is a trailer.
	saysTrailer = regexp.MustCompile(`(?i)\b(trailer|tr[aá]iler|avance)\b`)
	// The studio's own cut.
	saysOfficial = regexp.MustCompile(`(?i)\b(official|oficial)\b`)
	// Original audio with Spanish subtitles: exactly what is wanted here.
	saysSubtitled = regexp.MustCompile(`(?i)\b(subtitulad[oa]|subtitulos?|subt[ií]tulos?|vose|sub\s*esp)\b`)
	// Dubbed. Demoted rather than rejected, because for some films it is all
	// there is and a dubbed trailer beats no trailer.
	saysDubbed = regexp.MustCompile(`(?i)\b(doblad[oa]|espa[nñ]ol latino|latino|castellano|dublado|audio espa[nñ]ol)\b`)
)

// Pick chooses the best candidate for a film, or reports that none of them is
// one. reason says why, because a wrong trailer is silent and a person looking
// at the list later needs to be able to tell taste from a bug.
func Pick(cands []Candidate, title string, year int) (best Candidate, reason string, ok bool) {
	bestScore := 0
	for _, c := range cands {
		score, why := score(c, title, year)
		if score <= 0 {
			continue
		}
		if score > bestScore {
			best, bestScore, reason, ok = c, score, why, true
		}
	}
	return best, reason, ok
}

// score rates one candidate. Zero or less means "not this".
func score(c Candidate, title string, year int) (int, string) {
	if c.ID == "" {
		return 0, ""
	}
	if c.Duration > 0 && (c.Duration < minDuration || c.Duration > maxDuration) {
		return 0, ""
	}
	if notATrailer.MatchString(c.Title) {
		return 0, ""
	}

	n := 1
	var why []string

	if saysTrailer.MatchString(c.Title) {
		n += 3
		why = append(why, "dice trailer")
	}
	if saysOfficial.MatchString(c.Title) {
		n += 3
		why = append(why, "oficial")
	}
	switch {
	case c.Duration >= idealMin && c.Duration <= idealMax:
		n += 2
		why = append(why, "dura lo que dura un trailer")
	case c.Duration > 0:
		n++
	}

	// The preference that matters to this library: original audio, Spanish
	// subtitles. A dubbed trailer is still offered when nothing else scores,
	// which is why this is a penalty and not a rejection.
	if saysSubtitled.MatchString(c.Title) {
		n += 2
		why = append(why, "subtitulado")
	}
	if saysDubbed.MatchString(c.Title) {
		n -= 4
		why = append(why, "doblado")
	}

	// Not a penalty but a rejection when nothing matches at all. A video can
	// look like a perfect trailer — official, right length, says "trailer" —
	// and be a completely different film that the search happened to surface.
	// Those score well on everything else, so only this stops them.
	overlap := titleOverlap(c.Title, title)
	switch {
	case overlap == 0:
		return 0, ""
	case overlap >= 0.6:
		n += 2
		why = append(why, "coincide el título")
	case overlap < 0.3:
		n -= 3
	}
	if year > 0 && strings.Contains(c.Title, itoa(year)) {
		n++
	}
	return n, strings.Join(why, ", ")
}

// titleOverlap is the fraction of the film's significant words that appear in
// the video's title. Deliberately crude: it only has to tell "the right film"
// from "a different film that came up in the same search".
func titleOverlap(videoTitle, filmTitle string) float64 {
	want := words(filmTitle)
	if len(want) == 0 {
		return 1
	}
	have := make(map[string]bool, 16)
	for _, w := range words(videoTitle) {
		have[w] = true
	}
	var hit int
	for _, w := range want {
		if have[w] {
			hit++
		}
	}
	return float64(hit) / float64(len(want))
}

// words lowercases, strips accents and punctuation, and drops the short filler
// that matches everything.
func words(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		f = deaccent(f)
		if len(f) < 3 {
			continue
		}
		switch f {
		case "the", "and", "los", "las", "una", "unos", "unas", "del", "por", "con", "que":
			continue
		}
		out = append(out, f)
	}
	return out
}

var accents = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
	"à", "a", "è", "e", "ì", "i", "ò", "o", "ù", "u", "â", "a", "ê", "e",
	"î", "i", "ô", "o", "û", "u", "ç", "c",
)

func deaccent(s string) string { return accents.Replace(s) }

// itoa avoids pulling strconv in for one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
