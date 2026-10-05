package widgets

import (
	"context"
	"strconv"

	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/scanner"
	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/web/templates"
)

// DiskWidget shows the fullest watched disk. It uses a cheap statfs per folder
// (no recursive scan), so it is instant.
type DiskWidget struct {
	folders *folders.Store
}

// NewDiskWidget creates a DiskWidget backed by the given folder store.
func NewDiskWidget(fs *folders.Store) DiskWidget { return DiskWidget{folders: fs} }

func (DiskWidget) ID() string { return "disk" }

// Tile shows the fullest disk, because that is the one that runs out first.
func (w DiskWidget) Tile(ctx context.Context) templates.Tile {
	t := templates.Tile{Href: "/disk", Icon: "drive", Tone: "lilac", Title: "Disco"}
	list, err := w.folders.List(ctx, folders.PurposeDisk)
	if err != nil || len(list) == 0 {
		t.Value, t.Sub, t.Off = "—", "Sin carpetas configuradas", true
		return t
	}
	best := -1
	for _, f := range list {
		total, used, _, _, err := scanner.FilesystemStat(f.Path)
		if err != nil || total == 0 {
			continue
		}
		pct := int(float64(used) / float64(total) * 100)
		if pct > best {
			best = pct
			t.Value = strconv.Itoa(pct) + " %"
			t.Sub = system.HumanBytes(used) + " de " + system.HumanBytes(total) + " · " + f.Label
		}
	}
	if best < 0 {
		t.Value, t.Sub, t.Warn = "—", "No se pudo leer", true
		return t
	}
	t.Warn = best >= 90
	return t
}
