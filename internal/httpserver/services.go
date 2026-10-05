package httpserver

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/cristian/holocron/internal/services"
	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/web/templates"
)

func (s *Server) handleServicesPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.ServicesPage(servicesView(s.deps.Services.Current(r.Context()), s.deps.ServicesConfigured, time.Now())))
}

func (s *Server) handleServicesEvents(w http.ResponseWriter, r *http.Request) {
	streamLive(s, w, r, s.deps.Services, "services", func(v services.Snapshot) templ.Component {
		return templates.ServicesLive(servicesView(v, s.deps.ServicesConfigured, time.Now()))
	})
}

func servicesView(sn services.Snapshot, configured bool, now time.Time) templates.ServicesView {
	v := templates.ServicesView{Configured: configured, Errors: sn.Errors}
	for _, u := range sn.Units {
		su := templates.SvcUnit{Name: u.Name, OK: u.OK(), State: unitState(u)}
		if !u.Since.IsZero() {
			su.Since = "desde hace " + system.HumanDuration(now.Sub(u.Since))
		}
		if !su.OK {
			v.Down++
		}
		v.Units = append(v.Units, su)
	}
	for _, t := range sn.Timers {
		st := templates.SvcTimer{Name: t.Name, Last: "nunca", Next: "—"}
		if !t.Last.IsZero() {
			st.Last = ago(now.Sub(t.Last))
		}
		if !t.Next.IsZero() {
			st.Next = inDuration(t.Next.Sub(now))
		}
		// A oneshot that ran and finished is inactive with Result=success:
		// that is a job done, not a service down.
		switch t.Result {
		case "success":
			st.Result = "bien"
		case "":
			st.Result = "sin datos"
		default:
			st.Result, st.Failed = "falló ("+t.Result+")", true
		}
		v.Timers = append(v.Timers, st)
	}
	if d := sn.Drift; d != nil {
		sd := &templates.SvcDrift{
			Age:    "revisado " + ago(now.Sub(d.Generated)),
			Commit: strings.Fields(d.Commit + " ")[0],
			// Hourly by design: three hours without a report means the check
			// stopped, and "0 differences" from then is no longer news.
			Stale: now.Sub(d.Generated) > 3*time.Hour,
		}
		for _, it := range d.Differ {
			sd.Differ = append(sd.Differ, templates.SvcDiffer{File: it.File, Problem: it.Problem})
		}
		v.Drift = sd
	}
	if sm := sn.Smart; sm != nil {
		v.SmartAge = "medido " + ago(now.Sub(sm.Generated))
		for _, d := range sm.Disks {
			v.Disks = append(v.Disks, diskView(d))
		}
	}
	return v
}

func unitState(u services.Unit) string {
	switch u.Active {
	case "active":
		switch u.Sub {
		case "running":
			return "corriendo"
		case "mounted":
			return "montado"
		case "exited":
			return "activo"
		default:
			return u.Sub
		}
	case "failed":
		return "falló"
	case "inactive":
		return "parado"
	case "activating":
		return "arrancando"
	case "deactivating":
		return "parando"
	case "desconocido":
		return "no existe"
	default:
		return u.Active
	}
}

func inDuration(d time.Duration) string {
	if d <= 0 {
		return "ahora"
	}
	return "en " + system.HumanDuration(d)
}

// diskView summarises one SMART reading, and says out loud what is worth
// worrying about: a sector reallocated is a disk spending its spares, a
// pending one is a read that already failed once.
func diskView(d services.SmartDisk) templates.SvcDisk {
	out := templates.SvcDisk{Disk: strings.TrimPrefix(d.Disk, "/dev/"), Model: d.Model, Asleep: d.Asleep}
	if d.Asleep {
		return out
	}
	switch d.Health {
	case "ok":
		out.Health = "sano"
	case "falla":
		out.Health, out.Bad = "falla", true
	default:
		out.Health = "desconocido"
	}
	add := func(p *int64, f func(int64) string) {
		if p != nil {
			out.Facts = append(out.Facts, f(*p))
		}
	}
	add(d.TempC, func(v int64) string { return strconv.FormatInt(v, 10) + " °C" })
	add(d.Hours, func(v int64) string { return strconv.FormatInt(v, 10) + " h encendido" })
	add(d.WearPct, func(v int64) string { return "desgaste " + strconv.FormatInt(v, 10) + " %" })
	if d.Capacity > 0 {
		out.Facts = append(out.Facts, system.HumanBytes(uint64(d.Capacity)))
	}
	warn := func(p *int64, what string) {
		if p != nil && *p > 0 {
			out.Warn = append(out.Warn, fmt.Sprintf("%d %s", *p, what))
			out.Bad = true
		}
	}
	warn(d.Reallocated, "sectores reasignados")
	warn(d.Pending, "sectores pendientes")
	warn(d.Uncorrectable, "errores incorregibles")
	if d.CRC != nil && *d.CRC > 0 {
		// CRC errors are the cable or the USB bridge, not the platters.
		out.Warn = append(out.Warn, fmt.Sprintf("%d errores de CRC (cable o adaptador USB)", *d.CRC))
	}
	return out
}
