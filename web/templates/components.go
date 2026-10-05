package templates

import (
	"context"
	"strconv"
	"strings"
)

// navGroups are the sidebar's sections, in display order. A label doubles as
// the active-link key: it is matched against each page's title in Layout.
// Eleven links in one row read as a list to search; four groups say where
// each thing lives.
var navGroups = []struct {
	Label string
	Items []navItem
}{
	{"", []navItem{{"Inicio", "/", "home"}}},
	{"Ahora", []navItem{
		{"Actividad", "/activity", "activity"},
		{"Hardware", "/hardware", "cpu"},
		{"Servicios", "/services", "server"},
	}},
	{"Biblioteca", []navItem{
		{"Medios", "/media", "film"},
		{"Calidad", "/quality", "gauge"},
		{"Nombres", "/naming", "tag"},
		{"Disco", "/disk", "drive"},
	}},
	{"Descargas", []navItem{{"Torrents", "/torrents", "download"}}},
	{"Equipo", []navItem{
		{"Gestión", "/manage", "power"},
		{"Ajustes", "/settings", "gear"},
	}},
}

type navItem struct{ Label, Href, Icon string }

type muralKey struct{}

// WithMural carries the poster URLs for the wall behind the page. The layout
// reads them from the context so that no page has to pass them along.
func WithMural(ctx context.Context, urls []string) context.Context {
	return context.WithValue(ctx, muralKey{}, urls)
}

func muralFrom(ctx context.Context) []string {
	urls, _ := ctx.Value(muralKey{}).([]string)
	return urls
}

// AttnChip is one clickable pill in the dashboard's "Atención" strip.
type AttnChip struct {
	Label string `json:"label"`
	Href  string `json:"href"`
	Icon  string `json:"icon"` // sprite symbol id
}

// intToStr formats an int64 id for use in form values and URLs.
func intToStr(v int64) string { return strconv.FormatInt(v, 10) }

// yearStr renders a media year, blank when unknown (zero).
func yearStr(year int) string {
	if year == 0 {
		return "—"
	}
	return strconv.Itoa(year)
}

// clampPct constrains a percentage to the 0..100 range for use in bar widths.
func clampPct(p int) int {
	switch {
	case p < 0:
		return 0
	case p > 100:
		return 100
	default:
		return p
	}
}

// pwClass maps a percentage to a precomputed width class (.pw-0 … .pw-100).
// Bar widths are dynamic, but a strict CSP (no 'unsafe-inline' styles) forbids
// inline style attributes, so widths come from classes rather than style="".
func pwClass(p int) string { return "pw-" + strconv.Itoa(clampPct(p)) }

// filterIssues returns the naming issues of a given media type ("movies"/"tv"),
// so the page can group them by library.
func filterIssues(issues []NamingIssueRow, mediaType string) []NamingIssueRow {
	out := make([]NamingIssueRow, 0, len(issues))
	for _, is := range issues {
		if is.Type == mediaType {
			out = append(out, is)
		}
	}
	return out
}

// typeLabel is the short human badge for a media type.
func typeLabel(t string) string {
	switch t {
	case "movies":
		return "Peli"
	case "tv":
		return "Serie"
	default:
		return t
	}
}

// torrentClass maps a qBittorrent state to a status-pill class.
func torrentClass(state string, paused bool) string {
	s := strings.ToLower(state)
	switch {
	case paused:
		return "st-pause"
	case strings.Contains(s, "error") || strings.Contains(s, "missing"):
		return "st-err"
	case strings.Contains(s, "up"): // uploading, stalledUP, forcedUP, queuedUP
		return "st-seed"
	default:
		return "st-dl"
	}
}

// torrentLabel is the human status label matching torrentClass.
func torrentLabel(state string, paused bool) string {
	s := strings.ToLower(state)
	switch {
	case paused:
		return "Pausado"
	case strings.Contains(s, "error") || strings.Contains(s, "missing"):
		return "Error"
	case strings.Contains(s, "up"):
		return "Sembrando"
	default:
		return "Descargando"
	}
}

// Plural formats a count with a Spanish noun, picking singular or plural.
// "1 carpetas" is the kind of thing that makes an interface feel unfinished,
// and these counts are small often enough for it to show.
func Plural(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(n) + " " + plural
}

// noticeIcon picks the icon for a notice: an alert for an error, a tick
// otherwise. A function because templ has no ternary.
func noticeIcon(isErr bool) string {
	if isErr {
		return "alert"
	}
	return "check"
}

// initial is the first letter of a title, for a poster that has no image.
func initial(title string) string {
	for _, r := range strings.TrimSpace(title) {
		return strings.ToUpper(string(r))
	}
	return "·"
}

// typeLabelOf names an inventory row's type ("movie" / "show") in Spanish.
func typeLabelOf(t string) string {
	switch t {
	case "movie":
		return "Película"
	case "show":
		return "Serie"
	default:
		return t
	}
}

// liveOr is the reading once there is one, and "midiendo…" before.
func liveOr(live bool, v string) string {
	if !live {
		return "midiendo…"
	}
	return v
}

// dashIfEmpty stands in for a reading the machine does not have.
func dashIfEmpty(v string) string {
	if v == "" {
		return "—"
	}
	return v
}

// ofTotal is "used de total", or nothing when either is unknown.
func ofTotal(used, total string) string {
	if used == "" || total == "" {
		return ""
	}
	return used + " de " + total
}

// prefixed puts a label before a value, or returns nothing for no value.
func prefixed(label, v string) string {
	if v == "" {
		return ""
	}
	return label + v
}
