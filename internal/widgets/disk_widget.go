package widgets

import (
	"context"
	"strconv"

	"github.com/a-h/templ"

	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/scanner"
	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/web/templates"
)

// DiskWidget shows filesystem usage for each watched disk folder. It uses a
// cheap statfs per folder (no recursive scan), so rendering is instant.
type DiskWidget struct {
	folders *folders.Store
}

// NewDiskWidget creates a DiskWidget backed by the given folder store.
func NewDiskWidget(fs *folders.Store) DiskWidget { return DiskWidget{folders: fs} }

func (DiskWidget) ID() string    { return "disk" }
func (DiskWidget) Title() string { return "Disco" }

func (w DiskWidget) Card(ctx context.Context) templ.Component {
	chrome := templates.WidgetChrome{ID: w.ID(), Title: w.Title(), Icon: "drive", Span: "span-2"}
	view := templates.DiskWidgetView{}
	list, err := w.folders.List(ctx, folders.PurposeDisk)
	if err != nil || len(list) == 0 {
		view.Empty = true
		return templates.Widget(chrome, templates.DiskWidgetBody(view))
	}

	for _, f := range list {
		row := templates.DiskStatRow{
			Name: f.Label,
			Href: "/disk?folder=" + strconv.FormatInt(f.ID, 10),
		}
		total, used, _, _, err := scanner.FilesystemStat(f.Path)
		if err != nil {
			row.Err = "no se pudo leer"
		} else {
			row.TotalHuman = system.HumanBytes(total)
			row.UsedHuman = system.HumanBytes(used)
			if total > 0 {
				row.UsedPercent = int(float64(used) / float64(total) * 100)
				row.Hot = row.UsedPercent >= 90
			}
		}
		view.Rows = append(view.Rows, row)
	}
	return templates.Widget(chrome, templates.DiskWidgetBody(view))
}
