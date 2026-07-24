package templates

import (
	"context"
	"io"
	"strconv"
	"strings"

	"github.com/a-h/templ"
)

// navItems are the top-bar destinations, in display order. The label doubles as
// the active-link key: it is matched against each page's title in Layout.
var navItems = []struct{ Label, Href string }{
	{"Dashboard", "/"},
	{"Disco", "/disk"},
	{"Nombres", "/naming"},
	{"Medios", "/media"},
	{"Subtítulos", "/subtitles"},
	{"Torrents", "/torrents"},
	{"Ajustes", "/settings"},
}

// WidgetChrome carries the per-widget dashboard card presentation: its icon,
// grid span and whether it should read as an attention card (accent inset).
type WidgetChrome struct {
	ID    string
	Title string
	Icon  string // sprite symbol id
	Span  string // "", "span-2" or "span-4"
	Attn  bool
}

// AttnChip is one clickable pill in the dashboard's "Atención" strip.
type AttnChip struct {
	Label string
	Href  string
	Icon  string // sprite symbol id
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

// Grid renders the dashboard widget cards inside the grid container. The loop
// lives in Go rather than a templ range so the template stays a trivial shell.
func Grid(cards []templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := io.WriteString(w, `<div class="grid">`); err != nil {
			return err
		}
		for _, c := range cards {
			if err := c.Render(ctx, w); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, `</div>`)
		return err
	})
}
