// Package quality reports what is wrong with the library rather than how big it
// is. Holocron already counts titles and free space; none of that says that
// half the episodes have no Spanish subtitles, that hundreds of items have no
// synopsis, or that Jellyfin is listing files which no longer exist.
//
// Everything here is derived from one Jellyfin listing. The analysis is a pure
// function over that listing, so the rules can be tested against captured
// responses instead of against a running server.
package quality

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/cristian/holocron/internal/jellyfin"
)

// Kind is the job kind for a library audit.
const Kind = "quality-scan"

// maxPerCategory caps how many findings are kept for display. The counts are
// the real totals; a category with more than this says so in the UI rather than
// pretending the list is complete. Twelve hundred table rows would be
// unreadable on the page and slow to render on a Pi.
const maxPerCategory = 200

// Category is a kind of problem, which also decides what can be done about it.
type Category string

// The categories a report is grouped into.
const (
	// CatSubsMissing is an item with no Spanish subtitle track and no Spanish
	// audio. By far the largest category on a real library.
	CatSubsMissing Category = "subs-missing"
	// CatNoSynopsis is an item Jellyfin has no description for.
	CatNoSynopsis Category = "no-synopsis"
	// CatGenericTitle is a title that is filler or a release string rather than
	// a name — the sign of a failed metadata match.
	CatGenericTitle Category = "generic-title"
	// CatGhost is an item Jellyfin lists with no file behind it.
	CatGhost Category = "ghost"
	// CatCollision is two or more episodes claiming the same season/episode
	// number, which leaves one of them unreachable in most clients.
	CatCollision Category = "collision"
)

// Categories in display order.
var Categories = []Category{
	CatSubsMissing, CatNoSynopsis, CatGenericTitle, CatGhost, CatCollision,
}

// Label is the heading shown for the category.
func (c Category) Label() string {
	switch c {
	case CatSubsMissing:
		return "Sin subtítulos ES"
	case CatNoSynopsis:
		return "Sin sinopsis"
	case CatGenericTitle:
		return "Título genérico"
	case CatGhost:
		return "Fantasmas"
	case CatCollision:
		return "Numeración repetida"
	default:
		return string(c)
	}
}

// Hint says what to do about the category, because a counter nobody can act on
// is just bad news.
func (c Category) Hint() string {
	switch c {
	case CatSubsMissing:
		return "Buscalos en la pantalla de Subtítulos, o dejá el .srt junto al archivo."
	case CatNoSynopsis:
		return "Casi siempre alcanza con pedirle a Jellyfin que vuelva a leer la metadata."
	case CatGenericTitle:
		return "Jellyfin no pudo identificar el archivo. Revisá el nombre y refrescá la metadata."
	case CatGhost:
		return "El archivo ya no está: se limpian borrándolos desde Jellyfin o reescaneando la biblioteca ahí."
	case CatCollision:
		return "Dos archivos con el mismo SxxEyy: uno queda tapado. Se arregla renombrando."
	default:
		return ""
	}
}

// Refreshable reports whether asking Jellyfin to re-read the item's metadata is
// a plausible fix. Missing subtitles and a duplicated episode number are not
// metadata problems, so offering the button there would only spend a call on
// the metadata provider for nothing.
func (c Category) Refreshable() bool {
	return c == CatNoSynopsis || c == CatGenericTitle
}

// Finding is one problem with one item.
type Finding struct {
	Category Category `json:"category"`
	ItemID   string   `json:"itemId"`
	Title    string   `json:"title"`
	Detail   string   `json:"detail"`
	Path     string   `json:"path"`
	Kind     string   `json:"kind"` // película | serie | episodio
}

// Report is the cached result of one audit.
type Report struct {
	GeneratedAt time.Time        `json:"generatedAt"`
	Scanned     int              `json:"scanned"`
	Counts      map[Category]int `json:"counts"`
	Findings    []Finding        `json:"findings"`
}

// Count is how many items fall in the category, before any display cap.
func (r Report) Count(c Category) int { return r.Counts[c] }

// Total is how many findings the audit produced across every category.
func (r Report) Total() int {
	n := 0
	for _, c := range Categories {
		n += r.Counts[c]
	}
	return n
}

// For returns the findings kept for display in the category.
func (r Report) For(c Category) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Category == c {
			out = append(out, f)
		}
	}
	return out
}

// Truncated reports whether the category holds more findings than were kept.
func (r Report) Truncated(c Category) bool { return r.Counts[c] > len(r.For(c)) }

// Mentions reports whether the report refers to the item. Used to reject an
// item id that did not come from a finding: the id arrives from the browser and
// leads to a write on the media server.
func (r Report) Mentions(itemID string) bool {
	for _, f := range r.Findings {
		if f.ItemID == itemID {
			return true
		}
	}
	return false
}

// Analyse turns a Jellyfin listing into a report.
func Analyse(items []jellyfin.Item) Report {
	r := Report{Scanned: len(items), Counts: map[Category]int{}}

	type slot struct{ season, episode int }
	numbered := map[string]map[slot][]jellyfin.Item{}

	for _, it := range items {
		// A ghost is reported once, as a ghost. Piling "no synopsis" on top of
		// a file that does not exist buries the finding that matters.
		if !it.OnDisk() {
			r.add(CatGhost, it, "Jellyfin lo lista, pero no hay archivo en disco")
			continue
		}
		if strings.TrimSpace(it.Overview) == "" {
			r.add(CatNoSynopsis, it, "")
		}
		if why := genericTitle(it); why != "" {
			r.add(CatGenericTitle, it, why)
		}
		// A series is a directory: it carries no streams of its own, so asking
		// it about subtitles would report every single show as missing them.
		if it.Type != jellyfin.TypeSeries {
			switch it.SpanishSubtitleState() {
			case jellyfin.SubsMissing:
				r.add(CatSubsMissing, it, "Ninguna pista de subtítulos en español")
			case jellyfin.SubsUnknownLanguage:
				r.add(CatSubsMissing, it, "Hay un subtítulo sin idioma declarado")
			case jellyfin.SubsPresent, jellyfin.SubsNotNeeded:
			}
		}
		if it.Type == jellyfin.TypeEpisode && it.Season != nil && it.Episode != nil {
			key := slot{*it.Season, *it.Episode}
			if numbered[it.SeriesID] == nil {
				numbered[it.SeriesID] = map[slot][]jellyfin.Item{}
			}
			numbered[it.SeriesID][key] = append(numbered[it.SeriesID][key], it)
		}
	}

	// Collisions cannot be seen one item at a time: they need the whole season
	// in hand, so they are gathered after the pass above.
	for _, seasons := range numbered {
		for key, group := range seasons {
			if len(group) < 2 {
				continue
			}
			for _, it := range group {
				r.add(CatCollision, it, fmt.Sprintf("%s aparece en %d archivos",
					episodeCode(key.season, key.episode), len(group)))
			}
		}
	}

	r.finalise()
	return r
}

// add records a finding and counts it. Truncation happens later, in finalise,
// so which findings survive does not depend on map iteration order.
func (r *Report) add(c Category, it jellyfin.Item, detail string) {
	r.Counts[c]++
	r.Findings = append(r.Findings, Finding{
		Category: c,
		ItemID:   it.ID,
		Title:    displayTitle(it),
		Detail:   detail,
		Path:     it.Path,
		Kind:     kindLabel(it.Type),
	})
}

// finalise sorts the findings and caps each category. Sorting first is what
// makes two audits of an unchanged library produce the same report.
func (r *Report) finalise() {
	order := map[Category]int{}
	for i, c := range Categories {
		order[c] = i
	}
	slices.SortStableFunc(r.Findings, func(a, b Finding) int {
		if d := cmp.Compare(order[a.Category], order[b.Category]); d != 0 {
			return d
		}
		if d := cmp.Compare(a.Title, b.Title); d != 0 {
			return d
		}
		return cmp.Compare(a.Path, b.Path)
	})

	kept := map[Category]int{}
	out := make([]Finding, 0, len(r.Findings))
	for _, f := range r.Findings {
		if kept[f.Category] >= maxPerCategory {
			continue
		}
		kept[f.Category]++
		out = append(out, f)
	}
	r.Findings = out
}

// genericFiller matches the placeholder names a scraper leaves behind when it
// knows an episode exists but nothing about it.
var genericFiller = regexp.MustCompile(
	`(?i)^(episode|episodio|cap[íi]tulo|cap\.?|chapter|pel[íi]cula|movie|untitled|sin t[íi]tulo|unknown)\s*\d*$`)

// releaseSmell matches tokens that only appear in a release filename, never in
// a title someone would write. A title carrying one means Jellyfin gave up
// identifying the file and fell back to its name.
var releaseSmell = regexp.MustCompile(
	`(?i)(2160p|1080p|720p|480p|x264|x265|h\.?26[45]|hevc|web-?rip|web-?dl|blu-?ray|brrip|bdrip|hdtv|dvdrip|xvid|yify|rarbg|repack)`)

// dottedName matches a name using dots as word separators, which is a filename
// convention and not a title one.
var dottedName = regexp.MustCompile(`^[^\s]+\.[^\s]+\.[^\s]+\.[^\s]+$`)

// genericTitle explains why a title looks wrong, or returns "" if it looks fine.
func genericTitle(it jellyfin.Item) string {
	name := strings.TrimSpace(jellyfin.CleanDisplayTitle(it.Name))
	switch {
	case name == "":
		return "El ítem no tiene título"
	case genericFiller.MatchString(name):
		return "El título es un relleno, no un nombre"
	case releaseSmell.MatchString(name), dottedName.MatchString(name):
		return "El título es el nombre del archivo, no el del episodio"
	default:
		return ""
	}
}

// displayTitle names the item the way the panel shows it: an episode is
// meaningless without its series and its number.
func displayTitle(it jellyfin.Item) string {
	name := strings.TrimSpace(jellyfin.CleanDisplayTitle(it.Name))
	if name == "" {
		name = "(sin título)"
	}
	if it.Type != jellyfin.TypeEpisode {
		return name
	}
	parts := make([]string, 0, 3)
	if series := strings.TrimSpace(jellyfin.CleanDisplayTitle(it.SeriesName)); series != "" {
		parts = append(parts, series)
	}
	if it.Season != nil && it.Episode != nil {
		parts = append(parts, episodeCode(*it.Season, *it.Episode))
	}
	parts = append(parts, name)
	return strings.Join(parts, " · ")
}

func episodeCode(season, episode int) string {
	return fmt.Sprintf("S%02dE%02d", season, episode)
}

func kindLabel(itemType string) string {
	switch itemType {
	case jellyfin.TypeMovie:
		return "película"
	case jellyfin.TypeSeries:
		return "serie"
	case jellyfin.TypeEpisode:
		return "episodio"
	default:
		return strings.ToLower(itemType)
	}
}
