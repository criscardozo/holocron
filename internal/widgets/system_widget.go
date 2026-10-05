package widgets

import (
	"context"
	"fmt"

	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/web/templates"
)

// SystemWidget is the hardware tile: CPU first, then heat and memory.
type SystemWidget struct{}

func (SystemWidget) ID() string { return "system" }

// Tile reads a fresh snapshot. Unavailable metrics (e.g. when running
// off-device) render as "—".
func (SystemWidget) Tile(_ context.Context) templates.Tile {
	s := system.Read()
	v := SystemViewOf(s)
	t := templates.Tile{
		Href: "/hardware", Icon: "cpu", Tone: "violet", Title: "Hardware",
		Value: v.CPU,
		Sub:   v.Temp + " · RAM " + percentOrDash(s.MemTotal > 0, s.MemPercent),
	}
	t.Warn = s.HasTemp && s.TempC >= 80
	return t
}

// SystemViewOf formats a snapshot for display. Exported because the machine
// management screen shows the same figures, and two formatters would drift:
// one page would say "—" where the other said "0%".
func SystemViewOf(s system.Stats) templates.SystemView {
	return templates.SystemView{
		CPU:    percentOrDash(s.HasCPU, s.CPUPercent),
		RAM:    ramOrDash(s),
		Temp:   tempOrDash(s),
		Uptime: dash(s.HasUptime, system.HumanDuration(s.Uptime)),
		Load:   dash(s.HasLoad, fmt.Sprintf("%.2f", s.Load1)),
	}
}

func percentOrDash(ok bool, pct float64) string {
	if !ok {
		return "—"
	}
	return fmt.Sprintf("%.0f %%", pct)
}

func ramOrDash(s system.Stats) string {
	if s.MemTotal == 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f %% · %s / %s",
		s.MemPercent, system.HumanBytes(s.MemUsed), system.HumanBytes(s.MemTotal))
}

func tempOrDash(s system.Stats) string {
	if !s.HasTemp {
		return "—"
	}
	return fmt.Sprintf("%.0f °C", s.TempC)
}

func dash(ok bool, s string) string {
	if !ok {
		return "—"
	}
	return s
}
