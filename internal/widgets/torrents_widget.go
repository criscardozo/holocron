package widgets

import (
	"context"

	"github.com/a-h/templ"

	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/internal/torrents"
	"github.com/cristian/holocron/web/templates"
)

// TorrentsWidget shows qBittorrent activity: active/total and total speeds.
type TorrentsWidget struct {
	torrents *torrents.Service
}

// NewTorrentsWidget creates a TorrentsWidget.
func NewTorrentsWidget(s *torrents.Service) TorrentsWidget { return TorrentsWidget{torrents: s} }

func (TorrentsWidget) ID() string    { return "torrents" }
func (TorrentsWidget) Title() string { return "Torrents" }

func (w TorrentsWidget) Card(ctx context.Context) templ.Component {
	view := templates.TorrentsCardView{Configured: w.torrents.Configured(ctx)}
	if view.Configured {
		sum, err := w.torrents.Summary(ctx)
		if err != nil {
			view.Err = true
		} else {
			view.Total = sum.Total
			view.Active = sum.Active
			view.DlHuman = system.HumanBytes(uint64(sum.DlSpeed)) + "/s"
			view.UpHuman = system.HumanBytes(uint64(sum.UpSpeed)) + "/s"
		}
	}
	chrome := templates.WidgetChrome{ID: w.ID(), Title: w.Title(), Icon: "adown", Span: "span-4"}
	return templates.Widget(chrome, templates.TorrentsBody(view))
}
