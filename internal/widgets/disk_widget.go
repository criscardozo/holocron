package widgets

import (
	"context"
	"strconv"

	"github.com/cristian/holocron/internal/disks"
	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/web/templates"
)

// DiskWidget shows the fullest disk, or the first thing wrong with any of
// them. Everything it reads is statfs, mountinfo, sysfs and the SMART file:
// nothing that wakes a sleeping disk.
type DiskWidget struct {
	disks *disks.Watcher
}

// NewDiskWidget creates a DiskWidget over the server's disks.
func NewDiskWidget(w *disks.Watcher) DiskWidget { return DiskWidget{disks: w} }

func (DiskWidget) ID() string { return "disk" }

// Tile shows the fullest disk, because that is the one that runs out first.
// A missing mount, rising reallocated sectors or a library on another disk
// take the line instead: those are the things to go and look at.
func (w DiskWidget) Tile(ctx context.Context) templates.Tile {
	t := templates.Tile{Href: "/disk", Icon: "drive", Tone: "lilac", Title: "Disco"}
	rep := w.disks.Read(ctx)
	if len(rep.Disks) == 0 {
		t.Value, t.Sub, t.Off = "—", "Sin discos configurados", true
		return t
	}
	best := -1
	for _, d := range rep.Disks {
		if !d.StatOK || d.Total == 0 || !d.Mounted {
			continue
		}
		pct := d.UsedPercent()
		if pct > best {
			best = pct
			t.Value = strconv.Itoa(pct) + " %"
			t.Sub = system.HumanBytes(d.Free) + " libres · " + d.Name
		}
	}
	if best < 0 {
		t.Value, t.Sub, t.Warn = "—", "No se pudo leer", true
	}
	t.Rows = libraryRows(rep)
	if best >= 90 {
		t.Warn = true
	}
	if alerts := Alerts(rep); len(alerts) > 0 {
		t.Sub, t.Warn = alerts[0], true
	}
	return t
}

// Alerts are the disk problems worth a chip on the start page, in words.
func Alerts(rep disks.Report) []string {
	var out []string
	for _, d := range rep.Disks {
		if d.Checked && !d.Mounted {
			out = append(out, d.Name+" no está montado")
		}
		if d.ReallocUp() {
			out = append(out, d.Name+": suben los sectores reasignados")
		}
	}
	for _, m := range rep.Moves {
		out = append(out, m.Library+" pasó de "+m.From+" a "+m.To)
	}
	return out
}

// libraryRows are the disks the library lives on, each with its use: on
// Ginebra, Disco4 and Disco3 side by side. Without roles in the config, every
// disk. One disk alone keeps the plain value.
func libraryRows(rep disks.Report) []templates.TileRow {
	var lib, all []disks.Disk
	for _, d := range rep.Disks {
		if !d.StatOK || d.Total == 0 || !d.Mounted {
			continue
		}
		all = append(all, d)
		if d.Role == "biblioteca" {
			lib = append(lib, d)
		}
	}
	if len(lib) == 0 {
		lib = all
	}
	if len(lib) < 2 {
		return nil
	}
	rows := make([]templates.TileRow, 0, len(lib))
	for _, d := range lib {
		pct := d.UsedPercent()
		rows = append(rows, templates.TileRow{
			Label: d.Name, Value: strconv.Itoa(pct) + " %", Percent: pct, Hot: pct >= 90,
			Sub: system.HumanBytes(d.Free) + " libres",
		})
	}
	return rows
}
