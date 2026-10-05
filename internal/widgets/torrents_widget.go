package widgets

import (
	"context"

	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/internal/torrents"
	"github.com/cristian/holocron/web/templates"
)

// TorrentsWidget shows qBittorrent activity: active torrents and total speeds.
type TorrentsWidget struct {
	torrents *torrents.Service
}

// NewTorrentsWidget creates a TorrentsWidget.
func NewTorrentsWidget(s *torrents.Service) TorrentsWidget { return TorrentsWidget{torrents: s} }

func (TorrentsWidget) ID() string { return "torrents" }

// Tile is how many are moving, and how fast.
func (w TorrentsWidget) Tile(ctx context.Context) templates.Tile {
	t := templates.Tile{Href: "/torrents", Icon: "download", Tone: "mint", Title: "Torrents"}
	if !w.torrents.Configured(ctx) {
		t.Value, t.Sub, t.Off = "—", "qBittorrent sin configurar", true
		return t
	}
	sum, err := w.torrents.Summary(ctx)
	if err != nil {
		t.Value, t.Sub, t.Warn = "—", "qBittorrent no responde", true
		return t
	}
	t.Value = templates.Plural(sum.Active, "activo", "activos")
	t.Sub = "↓ " + system.HumanSignedBytes(sum.DlSpeed) + "/s · ↑ " +
		system.HumanSignedBytes(sum.UpSpeed) + "/s · " + templates.Plural(sum.Total, "en total", "en total")
	return t
}
