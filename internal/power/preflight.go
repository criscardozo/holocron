package power

import (
	"context"
	"fmt"
)

// Preflight is what would be interrupted right now. It exists because the
// question before pulling the machine down is never "are you sure" — it is
// "is anyone watching something", and that is checkable.
//
// Every field is best effort. A service that cannot be reached contributes
// nothing rather than blocking the screen: not knowing whether Jellyfin is busy
// is not a reason to refuse to show the buttons.
type Preflight struct {
	// Warnings name what is going on, one line each, ready to show.
	Warnings []string
	// Checked is false when nothing could be consulted at all, so the screen
	// can say it does not know instead of implying all clear.
	Checked bool
}

// Busy reports whether anything would be interrupted.
func (p Preflight) Busy() bool { return len(p.Warnings) > 0 }

// MediaProbe is what Preflight needs from Jellyfin. An interface so the check
// does not drag the whole library service in, and so it can be faked in tests.
type MediaProbe interface {
	// NowPlaying returns a line per session playing something.
	NowPlaying(ctx context.Context) ([]string, error)
	// RunningTasks returns the names of background tasks in progress.
	RunningTasks(ctx context.Context) ([]string, error)
}

// TorrentProbe is what Preflight needs from qBittorrent.
type TorrentProbe interface {
	// ActiveTorrents returns how many are downloading or seeding.
	ActiveTorrents(ctx context.Context) (int, error)
}

// Check asks what is in flight. Errors are swallowed on purpose: this runs on
// a page load and its job is to warn, not to fail. A probe that cannot answer
// simply does not contribute a warning, and Checked records whether any of them
// managed to.
func Check(ctx context.Context, media MediaProbe, torrents TorrentProbe) Preflight {
	var p Preflight

	if media != nil {
		if playing, err := media.NowPlaying(ctx); err == nil {
			p.Checked = true
			for _, what := range playing {
				p.Warnings = append(p.Warnings, "Se está reproduciendo "+what)
			}
		}
		if tasks, err := media.RunningTasks(ctx); err == nil {
			p.Checked = true
			for _, name := range tasks {
				p.Warnings = append(p.Warnings, "Jellyfin está corriendo «"+name+"»")
			}
		}
	}

	if torrents != nil {
		if active, err := torrents.ActiveTorrents(ctx); err == nil {
			p.Checked = true
			if active == 1 {
				p.Warnings = append(p.Warnings, "Hay 1 torrent activo")
			} else if active > 1 {
				p.Warnings = append(p.Warnings, fmt.Sprintf("Hay %d torrents activos", active))
			}
		}
	}

	return p
}
