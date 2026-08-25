package widgets

import (
	"context"

	"github.com/a-h/templ"

	"github.com/cristian/holocron/internal/quality"
	"github.com/cristian/holocron/web/templates"
)

// QualityWidget shows what the last library audit found. It never runs one:
// the audit reads the whole library from Jellyfin, which is not something a
// dashboard load should trigger.
type QualityWidget struct {
	quality *quality.Service
}

// NewQualityWidget creates a QualityWidget.
func NewQualityWidget(s *quality.Service) QualityWidget { return QualityWidget{quality: s} }

func (QualityWidget) ID() string    { return "quality" }
func (QualityWidget) Title() string { return "Calidad" }

func (w QualityWidget) Card(ctx context.Context) templ.Component {
	view := templates.QualityCardView{
		Configured: w.quality.Configured(ctx),
		Scanning:   w.quality.Scanning(),
	}
	if view.Configured {
		if report, ok, err := w.quality.Latest(ctx); err == nil && ok {
			view.HasReport = true
			view.Total = report.Total()
			for _, c := range quality.Categories {
				view.Counts = append(view.Counts, templates.QualityCount{
					Key:   string(c),
					Label: c.Label(),
					Count: report.Count(c),
					Href:  "/quality?cat=" + string(c),
				})
			}
		}
	}
	chrome := templates.WidgetChrome{
		ID: w.ID(), Title: w.Title(), Icon: "gauge", Span: "span-2",
		// Something to fix reads as needing attention; a clean library does not.
		Attn: view.Total > 0,
	}
	return templates.Widget(chrome, templates.QualityBody(view))
}
