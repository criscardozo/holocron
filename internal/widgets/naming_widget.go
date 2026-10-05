package widgets

import (
	"context"

	"github.com/cristian/holocron/internal/naming"
	"github.com/cristian/holocron/web/templates"
)

// NamingWidget shows how many media folders break the "Title (Year)"
// convention, from the cached count (no scan on page load).
type NamingWidget struct {
	naming *naming.Service
}

// NewNamingWidget creates a NamingWidget.
func NewNamingWidget(s *naming.Service) NamingWidget { return NamingWidget{naming: s} }

func (NamingWidget) ID() string { return "naming" }

// Tile is the count of folders to rename.
func (w NamingWidget) Tile(ctx context.Context) templates.Tile {
	t := templates.Tile{Href: "/naming", Icon: "tag", Tone: "sky", Title: "Nombres"}
	if !w.naming.HasMediaFolders(ctx) {
		t.Value, t.Sub, t.Off = "—", "Sin carpetas de medios", true
		return t
	}
	n, err := w.naming.Count(ctx)
	switch {
	case err != nil:
		t.Value, t.Sub, t.Warn = "—", "No se pudo leer", true
	case n == 0:
		t.Value, t.Sub = "Todo bien", "Todo cumple «Título (Año)»"
	default:
		t.Value = templates.Plural(n, "carpeta", "carpetas")
		t.Sub, t.Warn = "No cumplen «Título (Año)»", true
	}
	return t
}
