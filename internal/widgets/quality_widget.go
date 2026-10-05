package widgets

import (
	"context"

	"github.com/cristian/holocron/internal/quality"
	"github.com/cristian/holocron/web/templates"
)

// QualityWidget shows what the last library audit found. It never runs one:
// the audit reads the whole library from Jellyfin, which is not something a
// page load should trigger.
type QualityWidget struct {
	quality *quality.Service
}

// NewQualityWidget creates a QualityWidget.
func NewQualityWidget(s *quality.Service) QualityWidget { return QualityWidget{quality: s} }

func (QualityWidget) ID() string { return "quality" }

// Tile says how many findings there are, and in which category most of them.
func (w QualityWidget) Tile(ctx context.Context) templates.Tile {
	t := templates.Tile{Href: "/quality", Icon: "gauge", Tone: "amber", Title: "Calidad"}
	switch {
	case !w.quality.Configured(ctx):
		t.Value, t.Sub, t.Off = "—", "Jellyfin sin vincular", true
		return t
	case w.quality.Scanning():
		t.Value, t.Sub = "Analizando…", "La biblioteca entera, una vez"
		return t
	}
	report, ok, err := w.quality.Latest(ctx)
	switch {
	case err != nil:
		t.Value, t.Sub, t.Warn = "—", "No se pudo leer", true
	case !ok:
		t.Value, t.Sub = "Sin analizar", "Analizar la biblioteca"
	case report.Total() == 0:
		t.Value, t.Sub = "Todo bien", "Nada para revisar"
	default:
		t.Value = templates.Plural(report.Total(), "para revisar", "para revisar")
		var top quality.Category
		for _, c := range quality.Categories {
			if report.Count(c) > report.Count(top) {
				top = c
			}
		}
		if top != "" {
			t.Sub = "La mayoría: " + top.Label()
		}
		t.Warn = true
	}
	return t
}
