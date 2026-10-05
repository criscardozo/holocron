package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cristian/holocron/internal/activity"
	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/hardware"
	"github.com/cristian/holocron/internal/power"
	"github.com/cristian/holocron/internal/scanner"
	"github.com/cristian/holocron/internal/services"
	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/web/templates"
)

// homeRecent is how many recent additions the start page shows as posters.
const homeRecent = 10

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.Home(s.homeView(r.Context())))
}

// homeView reads everything the start page shows. The app's start screen is
// the same view, over the API.
func (s *Server) homeView(ctx context.Context) templates.HomeView {
	now := time.Now()

	// Everything the page needs comes from a different place; asked at once,
	// the page waits for the slowest instead of the sum.
	var (
		wg    sync.WaitGroup
		tiles []templates.Tile
		act   activity.Snapshot
		svc   services.Snapshot
		chips []templates.AttnChip
	)
	wg.Go(func() { tiles = s.deps.Widgets.Tiles(ctx) })
	wg.Go(func() { act = s.deps.Activity.Current(ctx) })
	wg.Go(func() { svc = s.deps.Services.Current(ctx) })
	wg.Go(func() { chips = s.attentionChips(ctx) })
	wg.Wait()

	av := activityView(act, now)
	v := templates.HomeView{
		Machine: power.MachineName(),
		Status:  homeStatus(system.Read(), hardware.ReadBattery()),
		Attn:    chips,
		Recent:  av.Recent,
	}
	if len(v.Recent) > homeRecent {
		v.Recent = v.Recent[:homeRecent]
	}

	// Activity leads and Services follows Hardware: what is happening, then
	// the machine, then what it is made of. The rest are the registry's.
	v.Tiles = append(v.Tiles, activityTile(av))
	if len(tiles) > 0 {
		v.Tiles = append(v.Tiles, tiles[0])
		tiles = tiles[1:]
	}
	v.Tiles = append(v.Tiles, servicesTile(servicesView(svc, s.deps.ServicesConfigured, now)))
	v.Tiles = append(v.Tiles, tiles...)
	return v
}

// homeStatus is the row of readings under the title.
func homeStatus(st system.Stats, b hardware.Battery) []templates.HomeStat {
	up := templates.HomeStat{Text: "En línea", Tone: "ok"}
	if st.HasUptime {
		up.Text += " · " + system.HumanDuration(st.Uptime)
	}
	out := []templates.HomeStat{up}
	if st.HasCPU {
		out = append(out, templates.HomeStat{Text: fmt.Sprintf("CPU %.0f %%", st.CPUPercent)})
	}
	if st.HasTemp {
		t := templates.HomeStat{Text: fmt.Sprintf("%.0f °C", st.TempC)}
		if st.TempC >= 80 {
			t.Tone = "warn"
		}
		out = append(out, t)
	}
	if st.MemTotal > 0 {
		out = append(out, templates.HomeStat{Text: fmt.Sprintf("RAM %.0f %%", st.MemPercent)})
	}
	if b.Present {
		t := templates.HomeStat{Text: fmt.Sprintf("Batería %d %%", b.Percent)}
		if b.Discharging() {
			t.Text += " · sin luz"
			t.Tone = "danger"
		}
		out = append(out, t)
	}
	return out
}

func activityTile(av templates.ActivityView) templates.Tile {
	t := templates.Tile{Href: "/activity", Icon: "activity", Tone: "pink", Title: "Actividad"}
	if !av.HasJellyfin && !av.HasTorrents && !av.HasArr && !av.HasSeerr {
		t.Value, t.Sub, t.Off = "—", "Nada configurado", true
		return t
	}
	t.Value = "Nadie mirando"
	if n := len(av.Playing); n > 0 {
		t.Value = templates.Plural(n, "reproducción", "reproducciones")
	}
	var sub []string
	if n := len(av.Downloads); n > 0 {
		sub = append(sub, templates.Plural(n, "bajando", "bajando"))
	}
	if av.ReqSummary != "" {
		sub = append(sub, av.ReqSummary)
	}
	if n := len(av.Upcoming); n > 0 {
		sub = append(sub, templates.Plural(n, "estreno en 30 días", "estrenos en 30 días"))
	}
	t.Sub = strings.Join(sub, " · ")
	t.Warn = len(av.Errors) > 0
	return t
}

func servicesTile(sv templates.ServicesView) templates.Tile {
	t := templates.Tile{Href: "/services", Icon: "server", Tone: "teal", Title: "Servicios"}
	if !sv.Configured {
		t.Value, t.Sub, t.Off = "—", "Nada para vigilar", true
		return t
	}
	t.Value = "Todo bien"
	if sv.Down > 0 {
		t.Value, t.Warn = templates.Plural(sv.Down, "caída", "caídas"), true
	}
	t.Sub = templates.Plural(len(sv.Units), "unidad", "unidades") + " · " +
		templates.Plural(len(sv.Timers), "tarea", "tareas")
	if len(sv.Errors) > 0 {
		t.Warn = true
	}
	return t
}

// attentionChips builds the start page's "Atención" strip: one chip per thing
// that needs action (a power cut, invalid folder names, disks that are nearly
// full). All lookups are best-effort — a failing service just
// omits its chip rather than breaking the dashboard.
func (s *Server) attentionChips(ctx context.Context) []templates.AttnChip {
	var chips []templates.AttnChip

	// First, because it is the only chip with a clock running. On Ginebra the
	// laptop's battery is the UPS: a power cut shows here on every screen the
	// dashboard is, not only on /hardware where nobody might be looking.
	if b := hardware.ReadBattery(); b.Discharging() {
		label := fmt.Sprintf("Sin luz: batería al %d %%", b.Percent)
		if b.Left > 0 {
			label += ", quedan unos " + system.HumanDuration(b.Left)
		}
		chips = append(chips, templates.AttnChip{Label: label, Href: "/hardware", Icon: "power"})
	}

	if n, err := s.deps.Naming.Count(ctx); err == nil && n > 0 {
		chips = append(chips, templates.AttnChip{
			Label: fmt.Sprintf("%d nombres inválidos", n),
			Href:  "/naming",
			Icon:  "alert",
		})
	}

	if list, err := s.deps.Folders.List(ctx, folders.PurposeDisk); err == nil {
		for _, f := range list {
			total, used, _, _, err := scanner.FilesystemStat(f.Path)
			if err != nil || total == 0 {
				continue
			}
			if pct := int(float64(used) / float64(total) * 100); pct >= 90 {
				chips = append(chips, templates.AttnChip{
					Label: fmt.Sprintf("%s %d%%", f.Label, pct),
					Href:  "/disk",
					Icon:  "drive",
				})
			}
		}
	}

	return chips
}
