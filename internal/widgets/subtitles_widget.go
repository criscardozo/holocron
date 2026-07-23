package widgets

import (
	"context"

	"github.com/a-h/templ"

	"github.com/cristian/holocron/internal/subtitles"
	"github.com/cristian/holocron/web/templates"
)

// SubtitlesWidget shows how many media items lack a Spanish subtitle.
type SubtitlesWidget struct {
	subtitles *subtitles.Service
}

// NewSubtitlesWidget creates a SubtitlesWidget.
func NewSubtitlesWidget(s *subtitles.Service) SubtitlesWidget { return SubtitlesWidget{subtitles: s} }

func (SubtitlesWidget) ID() string    { return "subtitles" }
func (SubtitlesWidget) Title() string { return "Subtítulos" }

func (w SubtitlesWidget) Card(ctx context.Context) templ.Component {
	view := templates.SubtitlesCardView{Configured: w.subtitles.Configured(ctx)}
	if count, err := w.subtitles.MissingCount(ctx); err == nil {
		view.Missing = count
	}
	return templates.Widget(w.ID(), w.Title(), templates.SubtitlesBody(view))
}
