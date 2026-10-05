package widgets

import (
	"context"
	"strconv"

	"github.com/cristian/holocron/internal/library"
	"github.com/cristian/holocron/web/templates"
)

// MediaWidget shows the size of the media inventory.
type MediaWidget struct {
	library *library.Service
}

// NewMediaWidget creates a MediaWidget.
func NewMediaWidget(s *library.Service) MediaWidget { return MediaWidget{library: s} }

func (MediaWidget) ID() string { return "media" }

// Tile counts films and series. Subtitles are not on it: Bazarr owns them now,
// and its count is on Actividad, named as Bazarr's.
func (w MediaWidget) Tile(ctx context.Context) templates.Tile {
	t := templates.Tile{Href: "/media", Icon: "film", Tone: "indigo", Title: "Medios"}
	if !w.library.Configured(ctx) {
		t.Value, t.Sub, t.Off = "—", "Jellyfin sin vincular", true
		return t
	}
	st, err := w.library.Stats(ctx)
	if err != nil {
		t.Value, t.Sub, t.Warn = "—", "No se pudo leer", true
		return t
	}
	t.Value = strconv.Itoa(st.Total)
	t.Sub = templates.Plural(st.Movies, "película", "películas") + " · " +
		templates.Plural(st.Total-st.Movies, "serie", "series")
	return t
}
