package httpserver

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cristian/holocron/internal/power"
	"github.com/cristian/holocron/internal/serverupdates"
	"github.com/cristian/holocron/web/templates"
)

// The Actualizaciones panel shows what the server says about its own updates
// (HOLOCRON_UPDATES_FILE) and asks it to look again. Holocron neither checks
// nor installs anything: see internal/serverupdates.

// checkTimeout is how long the panel waits for the server to rewrite the file
// after the button. A check takes 10–60 s; past this something is off, and
// polling forever would hide that.
const checkTimeout = 3 * time.Minute

// updatesCheck is a check under way, carried in the polling URL rather than
// kept on the server: since is the file's "generado" when the button was
// pressed, started when it was pressed, both in Unix seconds.
type updatesCheck struct{ since, started int64 }

func (s *Server) handleServerUpdates(w http.ResponseWriter, r *http.Request) {
	var check *updatesCheck
	since, errSince := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	started, errStarted := strconv.ParseInt(r.URL.Query().Get("started"), 10, 64)
	if errSince == nil && errStarted == nil {
		check = &updatesCheck{since: since, started: started}
	}
	s.render(w, r, templates.ServerUpdates(s.serverUpdatesView(time.Now(), check)))
}

// handleServerUpdatesCheck drops the trigger. No confirmation and no token:
// looking interrupts nothing, and the server never installs on this request.
func (s *Server) handleServerUpdatesCheck(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	svc := s.deps.ServerUpdates
	if svc == nil || !svc.Configured() {
		http.NotFound(w, r)
		return
	}
	// What the file says now, so the panel can tell when it has been
	// rewritten. Unreadable counts as never written: any new file is news.
	var since int64
	if st, err := svc.Read(); err == nil {
		since = st.Generated.Unix()
	}
	if err := svc.RequestCheck(); err != nil {
		s.log.Warn("request update check", "error", err)
		v := s.serverUpdatesView(now, nil)
		v.NoticeErr = true
		v.Notice = "No se pudo pedir la revisión."
		if errors.Is(err, serverupdates.ErrNoHelper) {
			v.Notice = "El servidor no tiene instalado lo que hace la revisión (holocron-check-updates.path)."
		}
		s.render(w, r, templates.ServerUpdates(v))
		return
	}
	s.log.Info("update check requested")
	s.render(w, r, templates.ServerUpdates(s.serverUpdatesView(now, &updatesCheck{since: since, started: now.Unix()})))
}

// serverUpdatesView assembles the panel. check is a check the browser is
// waiting on, or nil.
func (s *Server) serverUpdatesView(now time.Time, check *updatesCheck) templates.ServerUpdatesView {
	v := templates.ServerUpdatesView{Machine: power.MachineName()}
	svc := s.deps.ServerUpdates
	if svc == nil || !svc.Configured() {
		return v
	}
	st, err := svc.Read()
	switch {
	case errors.Is(err, os.ErrNotExist):
		// Not written yet, or not this server: no panel rather than an error.
		return v
	case err != nil:
		s.log.Warn("read server updates", "error", err)
		v.Show = true
		v.Error = "No se pudo leer el estado de las actualizaciones."
		v.CanCheck = svc.CheckAvailable()
		return v
	}
	v.Show = true
	if !st.LastUpdate.IsZero() {
		v.LastUpdate = whenPast(st.LastUpdate, now)
	}
	if !st.Next.IsZero() {
		v.Next = whenNext(st.Next, now)
	}
	if !st.Generated.IsZero() {
		v.Checked, _ = agoPhrase(now.Sub(st.Generated))
		if v.Checked == "" {
			v.Checked = "el " + longDate(st.Generated)
		}
	}
	v.Running = st.Running
	v.RebootPending = st.RebootPending
	for _, a := range st.Apps {
		row := updateRow(a)
		switch a.State {
		case serverupdates.StateNew:
			v.New++
		case serverupdates.StateMajor:
			v.Major++
		case serverupdates.StateError:
			v.Failed++
		}
		v.Apps = append(v.Apps, row)
	}
	if st.LastRun != nil {
		run := &templates.ServerUpdateRun{Title: "Última vuelta", Failures: st.LastRun.Failures}
		if !st.LastRun.At.IsZero() {
			run.Title += ", " + whenPast(st.LastRun.At, now)
		}
		if n := len(st.LastRun.Changes); n > 0 {
			run.Title += ": " + templates.Plural(n, "cambio", "cambios")
		}
		for _, c := range st.LastRun.Changes {
			run.Changes = append(run.Changes, templates.ServerUpdateChange{
				Text: c, Failed: strings.Contains(strings.ToUpper(c), "FALLO"),
			})
		}
		v.LastRun = run
	}

	// A request made before this page was loaded and not yet picked up.
	if check == nil {
		if at, ok := svc.CheckPending(); ok {
			check = &updatesCheck{since: st.Generated.Unix(), started: at.Unix()}
		}
	}
	switch {
	case st.Running:
		// The install rewrites the file when it ends, which also answers any
		// check asked for meanwhile: the server will not look while it
		// installs.
		v.PollURL = "/manage/updates"
	case check != nil && st.Generated.Unix() <= check.since:
		if elapsed := now.Sub(time.Unix(check.started, 0)); elapsed >= 0 && elapsed < checkTimeout {
			v.Checking = true
			v.PollURL = fmt.Sprintf("/manage/updates?since=%d&started=%d", check.since, check.started)
		} else {
			v.CheckSlow = true
		}
	}
	v.CanCheck = svc.CheckAvailable() && !v.Running && !v.Checking
	return v
}

// updateRow is one app as the panel shows it.
func updateRow(a serverupdates.App) templates.ServerUpdateRow {
	row := templates.ServerUpdateRow{Name: a.Name, Installed: a.Installed, Note: a.Note}
	if a.Latest != a.Installed {
		row.Latest = a.Latest
	}
	switch a.State {
	case serverupdates.StateCurrent:
		// Quiet on purpose: fifteen green badges would say nothing.
		row.Badge, row.BadgeClass = "al día", "badge-neutral"
	case serverupdates.StateNew:
		row.Badge, row.BadgeClass, row.Attention = "versión nueva", "badge-warn", true
	case serverupdates.StateMajor:
		row.Badge, row.BadgeClass, row.Attention = "versión mayor: a mano", "badge-aviso", true
	default:
		row.Badge, row.BadgeClass, row.Attention = "no se pudo revisar", "badge-no", true
		// What it failed to find is not a version to show.
		row.Latest = ""
	}
	return row
}

var monthNames = [...]string{
	"enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
}

// longDate is "29 de octubre", in the server's time zone.
func longDate(t time.Time) string {
	t = t.Local()
	return strconv.Itoa(t.Day()) + " de " + monthNames[t.Month()-1]
}

// calendarDays counts the days between two dates as a calendar does, so
// tomorrow at 04:30 is one day away even when it is only six hours.
func calendarDays(from, to time.Time) int {
	from, to = from.Local(), to.Local()
	a := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	b := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(b.Sub(a).Hours() / 24)
}

// whenPast says when something already happened: "hoy a las 10:30", "ayer a
// las 10:30", "hace 3 días (10 de octubre)".
func whenPast(t, now time.Time) string {
	clock := t.Local().Format("15:04")
	switch days := calendarDays(t, now); {
	case days <= 0:
		return "hoy a las " + clock
	case days == 1:
		return "ayer a las " + clock
	default:
		return "hace " + strconv.Itoa(days) + " días (" + longDate(t) + ")"
	}
}

// whenNext says when something will happen: "hoy a las 04:30", "mañana a las
// 04:30", "en 16 días (29 de octubre)". A date already gone means the run is
// overdue, most likely because the machine was off at the time.
func whenNext(t, now time.Time) string {
	if t.Before(now) {
		return "pendiente (tocaba el " + longDate(t) + ")"
	}
	clock := t.Local().Format("15:04")
	switch days := calendarDays(now, t); {
	case days <= 0:
		return "hoy a las " + clock
	case days == 1:
		return "mañana a las " + clock
	default:
		return "en " + strconv.Itoa(days) + " días (" + longDate(t) + ")"
	}
}
