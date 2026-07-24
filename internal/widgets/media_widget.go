package widgets

import (
	"context"

	"github.com/a-h/templ"

	"github.com/cristian/holocron/internal/library"
	"github.com/cristian/holocron/web/templates"
)

// MediaWidget shows the media inventory summary (total, with .nfo, without
// Spanish subtitles).
type MediaWidget struct {
	library *library.Service
}

// NewMediaWidget creates a MediaWidget.
func NewMediaWidget(s *library.Service) MediaWidget { return MediaWidget{library: s} }

func (MediaWidget) ID() string    { return "media" }
func (MediaWidget) Title() string { return "Medios" }

func (w MediaWidget) Card(ctx context.Context) templ.Component {
	view := templates.MediaCardView{Configured: w.library.Configured(ctx)}
	if view.Configured {
		if st, err := w.library.Stats(ctx); err == nil {
			view.Total = st.Total
			view.WithNFO = st.WithNFO
			view.WithoutSubs = st.WithoutSubs
		}
	}
	chrome := templates.WidgetChrome{ID: w.ID(), Title: w.Title(), Icon: "film", Span: "span-2"}
	return templates.Widget(chrome, templates.MediaBody(view))
}
