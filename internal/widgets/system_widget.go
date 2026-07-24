package widgets

import (
	"context"
	"fmt"

	"github.com/a-h/templ"

	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/web/templates"
)

// SystemWidget shows Raspberry Pi health: CPU, RAM, temperature, uptime, load.
type SystemWidget struct{}

func (SystemWidget) ID() string    { return "system" }
func (SystemWidget) Title() string { return "Sistema" }

// Card reads a fresh snapshot and renders it. Unavailable metrics (e.g. when
// running off-device) render as "—".
func (w SystemWidget) Card(_ context.Context) templ.Component {
	s := system.Read()
	v := templates.SystemView{
		CPU:    percentOrDash(s.HasCPU, s.CPUPercent),
		RAM:    ramOrDash(s),
		Temp:   tempOrDash(s),
		Uptime: dash(s.HasUptime, system.HumanDuration(s.Uptime)),
		Load:   dash(s.HasLoad, fmt.Sprintf("%.2f", s.Load1)),
	}
	chrome := templates.WidgetChrome{ID: w.ID(), Title: w.Title(), Icon: "activity", Span: "span-2"}
	return templates.Widget(chrome, templates.SystemBody(v))
}

func percentOrDash(ok bool, pct float64) string {
	if !ok {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", pct)
}

func ramOrDash(s system.Stats) string {
	if s.MemTotal == 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f%% · %s / %s",
		s.MemPercent, system.HumanBytes(s.MemUsed), system.HumanBytes(s.MemTotal))
}

func tempOrDash(s system.Stats) string {
	if !s.HasTemp {
		return "—"
	}
	return fmt.Sprintf("%.1f °C", s.TempC)
}

func dash(ok bool, s string) string {
	if !ok {
		return "—"
	}
	return s
}
