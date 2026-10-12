package httpserver

import (
	"context"
	"errors"
	"fmt"
	"github.com/cristian/holocron/internal/disks"
	"github.com/cristian/holocron/internal/stack"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/scanner"
	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/web/templates"
)

func (s *Server) handleDiskPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rep := s.deps.Disks.Read(ctx)
	view := templates.DiskPageView{}
	if len(rep.Disks) == 0 {
		view.Empty = true
		s.render(w, r, templates.DiskPage(view))
		return
	}

	var selID int64
	if q := r.URL.Query().Get("folder"); q != "" {
		if id, err := strconv.ParseInt(q, 10, 64); err == nil {
			selID = id
		}
	}
	var selected *disks.Disk
	for i := range rep.Disks {
		d := &rep.Disks[i]
		if selected == nil && d.FolderID != 0 && (selID == 0 || d.FolderID == selID) {
			selected = d
		}
	}
	now := time.Now()
	for _, d := range rep.Disks {
		view.Cards = append(view.Cards, diskCard(d, selected != nil && d.FolderID == selected.FolderID, now))
	}
	view.Alerts = s.diskAlerts(ctx, rep)

	if selected != nil {
		if f, err := s.deps.Folders.Get(ctx, selected.FolderID); err == nil {
			detail := s.buildDiskDetail(ctx, f, *selected)
			view.Selected = &detail
		}
	}
	s.render(w, r, templates.DiskPage(view))
}

// diskCard is one disk at a glance.
func diskCard(d disks.Disk, active bool, now time.Time) templates.DiskCard {
	c := templates.DiskCard{
		Name: d.Name, Model: d.Model, Role: d.Role, Active: active,
		Checked: d.Checked, Mounted: d.Mounted, StatOK: d.StatOK,
		Libraries: strings.Join(d.Libraries, " · "),
	}
	if d.FolderID != 0 {
		c.Href = "/disk?folder=" + strconv.FormatInt(d.FolderID, 10)
	}
	if d.StatOK && d.Total > 0 {
		c.UsedHuman = system.HumanBytes(d.Used)
		c.TotalHuman = system.HumanBytes(d.Total)
		c.FreeHuman = system.HumanBytes(d.Free)
		c.UsedPercent = int(float64(d.Used) / float64(d.Total) * 100)
	}

	sd := d.Smart
	if sd == nil {
		// Mounted and found, but nothing about it in the SMART file: a bridge
		// that does not pass the commands (Disco4's) reads like this. Not a
		// failure, an absence.
		if d.Device != "" {
			c.Health, c.HealthTone = "sin datos de salud", "none"
		}
		return c
	}
	c.Asleep = sd.Asleep
	age := ""
	if sd.Asleep && sd.ReadAt != nil {
		age = "leído " + ago(now.Sub(*sd.ReadAt))
	}
	if sd.TempC != nil {
		c.Temp = strconv.FormatInt(*sd.TempC, 10) + " °C"
		c.TempAge = age
	}
	switch sd.Health {
	case "ok":
		c.Health, c.HealthTone = "sano", "ok"
	case "falla":
		c.Health, c.HealthTone = "falla", "bad"
	default:
		c.Health, c.HealthTone = "sin datos de salud", "none"
	}
	if sd.Reallocated != nil {
		c.Realloc = strconv.FormatInt(*sd.Reallocated, 10) + " reasignados"
		if age != "" {
			c.Realloc += " · " + age
		}
		c.ReallocUp = d.ReallocUp()
	}
	return c
}

// diskAlerts are the disk problems on the page, each with what settles it.
func (s *Server) diskAlerts(ctx context.Context, rep disks.Report) []templates.DiskAlert {
	var out []templates.DiskAlert
	for _, d := range rep.Disks {
		if d.Checked && !d.Mounted {
			out = append(out, templates.DiskAlert{
				Text: d.Name + " no está montado.", Bad: true,
				Detail: "Lo que guarda no está disponible, y la carpeta donde se monta puede mostrar otra cosa en su lugar.",
			})
		}
		if d.ReallocUp() {
			out = append(out, templates.DiskAlert{
				Text: fmt.Sprintf("%s: los sectores reasignados subieron de %d a %d.", d.Name, *d.Baseline, *d.Smart.Reallocated),
				Detail: "Es el primer aviso de un disco que se está gastando: conviene tener copia de lo que guarda. " +
					"Si ya lo sabés, tomalo como el nuevo normal y el aviso vuelve sólo si sigue subiendo.",
				Action: "/disk/accept-realloc", Field: "serial", Value: d.Smart.Serial, Button: "Tomar como nuevo normal",
			})
		}
	}
	if len(rep.Moves) > 0 {
		var parts []string
		for _, m := range rep.Moves {
			parts = append(parts, m.Library+" pasó de "+m.From+" a "+m.To)
		}
		detail := "Si no lo moviste vos, puede faltar un montaje, y lo que se ve ahí puede ser una copia vieja."
		if stopped := s.stoppedApps(ctx); len(stopped) > 0 {
			detail += " Y no están andando: " + strings.Join(stopped, ", ") + "."
		}
		out = append(out, templates.DiskAlert{
			Text: strings.Join(parts, "; ") + ".", Detail: detail,
			Action: "/disk/accept-layout", Button: "Es correcto",
		})
	}
	return out
}

// stoppedApps are the stack's apps whose unit is watched and not running:
// after a missing bind, the ones that require that mount.
func (s *Server) stoppedApps(ctx context.Context) []string {
	if s.deps.Services == nil || !s.deps.ServicesConfigured {
		return nil
	}
	down := map[string]bool{}
	for _, u := range s.deps.Services.Current(ctx).Units {
		if !u.OK() {
			down[u.Name] = true
		}
	}
	var out []string
	for _, a := range stack.Apps {
		if down[a.Unit] {
			out = append(out, a.Name)
		}
	}
	return out
}

func (s *Server) handleDiskAcceptLayout(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Disks.AcceptLayout(r.Context()); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, "/disk")
}

func (s *Server) handleDiskAcceptRealloc(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if err := s.deps.Disks.AcceptRealloc(r.Context(), r.PostFormValue("serial")); err != nil {
		s.log.Warn("accept reallocated count", "error", err)
		http.Error(w, "unknown disk", http.StatusBadRequest)
		return
	}
	s.redirect(w, r, "/disk")
}

// hiddenFloor is how much unexplained use is still the filesystem's own:
// journal, inode tables, lost+found. Below the larger of 1 % of the disk and
// 20 GB it is not reported.
func hiddenFloor(total uint64) uint64 {
	const twentyGB = 20 << 30
	if onePct := total / 100; onePct > twentyGB {
		return onePct
	}
	return twentyGB
}

func (s *Server) buildDiskDetail(ctx context.Context, f folders.Folder, disk disks.Disk) templates.DiskDetail {
	d := templates.DiskDetail{
		FolderID: strconv.FormatInt(f.ID, 10),
		Label:    f.Label,
		Path:     f.Path,
		Scanning: s.deps.Disk.Scanning(f.ID),
	}

	if total, used, free, _, err := scanner.FilesystemStat(f.Path); err == nil {
		d.TotalHuman = system.HumanBytes(total)
		d.UsedHuman = system.HumanBytes(used)
		d.FreeHuman = system.HumanBytes(free)
		if total > 0 {
			d.UsedPercent = int(float64(used) / float64(total) * 100)
		}
	}

	if res, scannedAt, ok, err := s.deps.Disk.CachedResult(ctx, f.ID); err == nil && ok {
		d.HasResult = true
		d.ScannedAt = humanScannedAt(scannedAt)
		var maxTop uint64
		for _, t := range res.TopFolders {
			if t.Bytes > maxTop {
				maxTop = t.Bytes
			}
		}
		for _, t := range res.TopFolders {
			pct := 0
			if maxTop > 0 {
				pct = int(float64(t.Bytes) / float64(maxTop) * 100)
			}
			d.Top = append(d.Top, templates.DiskTopRow{
				Name:       t.Name,
				BytesHuman: system.HumanBytes(t.Bytes),
				Percent:    pct,
				BrowseHref: browseHref(f.ID, t.Path),
			})
		}
		var visible uint64
		for _, t := range res.TopFolders {
			visible += t.Bytes
		}
		for _, e := range res.Errors {
			// lost+found is root's; not reading it is expected, not a failure.
			if strings.Contains(strings.ToLower(e.Message), "permission denied") {
				d.NoAccess = append(d.NoAccess, e.Path)
				continue
			}
			d.Errors = append(d.Errors, e.Path+": "+e.Message)
		}
		// Use the folders do not account for. Only when the scan covers the
		// whole disk: on the system disk only the downloads are scanned, and
		// the rest of the system is not "hidden".
		if disk.Scan == disk.Mount && disk.StatOK && disk.Used > visible && disk.Used-visible > hiddenFloor(disk.Total) {
			d.Hidden = system.HumanBytes(disk.Used - visible)
		}
	}
	return d
}

func (s *Server) handleDiskScan(w http.ResponseWriter, r *http.Request) {
	id, ok := s.formInt64(w, r, "folder")
	if !ok {
		return
	}
	_, err := s.deps.Disk.StartScan(r.Context(), id)
	if err != nil && !errors.Is(err, jobs.ErrKindBusy) {
		s.log.Warn("start scan", "folder", id, "error", err)
		s.render(w, r, templates.ScanStatus(templates.ScanStatusView{Error: "No se pudo iniciar el escaneo."}))
		return
	}
	s.render(w, r, templates.ScanStatus(templates.ScanStatusView{
		Running:    true,
		StatusHref: "/disk/status?folder=" + strconv.FormatInt(id, 10),
	}))
}

func (s *Server) handleDiskStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := queryInt64(r, "folder")
	if !ok {
		http.Error(w, "bad folder", http.StatusBadRequest)
		return
	}
	if s.deps.Disk.Scanning(id) {
		s.render(w, r, templates.ScanStatus(templates.ScanStatusView{
			Running:    true,
			StatusHref: "/disk/status?folder=" + strconv.FormatInt(id, 10),
		}))
		return
	}
	view := templates.ScanStatusView{ReloadHref: "/disk?folder=" + strconv.FormatInt(id, 10)}
	if job, ok := s.deps.Disk.LastJob(id); ok {
		if job.Status == jobs.StatusError {
			view.Error = "El escaneo falló."
		} else {
			view.Summary = job.Result
		}
	}
	s.render(w, r, templates.ScanStatus(view))
}

func (s *Server) handleDiskBrowse(w http.ResponseWriter, r *http.Request) {
	id, ok := queryInt64(r, "folder")
	if !ok {
		http.Error(w, "bad folder", http.StatusBadRequest)
		return
	}
	res, err := s.deps.Disk.Browse(r.Context(), id, r.URL.Query().Get("path"))
	if err != nil {
		s.log.Warn("browse", "folder", id, "error", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<p class="error">No se pudo explorar esta carpeta.</p>`))
		return
	}

	view := templates.BrowseView{
		Path:       res.Path,
		TotalHuman: system.HumanBytes(res.TotalBytes),
	}
	if res.Parent != "" {
		view.HasParent = true
		view.ParentHref = browseHref(id, res.Parent)
	}
	var maxBytes uint64
	for _, e := range res.Entries {
		if e.Bytes > maxBytes {
			maxBytes = e.Bytes
		}
	}
	// Which disk a foreign folder is: the configured disk mounted on the
	// same device. On Ginebra, Peliculas under Disco4 is Disco3.
	diskOnDev := map[uint64]disks.Disk{}
	for _, d := range s.deps.Disks.Read(r.Context()).Disks {
		if dev, ok := disks.DevOf(d.Mount); ok && d.Mounted {
			diskOnDev[dev] = d
		}
	}
	for _, e := range res.Entries {
		row := templates.BrowseRow{
			Name:       e.Name,
			BytesHuman: system.HumanBytes(e.Bytes),
			IsDir:      e.IsDir,
		}
		if e.Foreign {
			row.Foreign = true
			row.ForeignName = "otro disco"
			if dev, ok := disks.DevOf(e.Path); ok {
				if d, ok := diskOnDev[dev]; ok {
					row.ForeignName = d.Name
					if d.FolderID != 0 {
						row.ForeignHref = "/disk?folder=" + strconv.FormatInt(d.FolderID, 10)
					}
				}
			}
			view.Entries = append(view.Entries, row)
			continue
		}
		if maxBytes > 0 {
			row.Percent = int(float64(e.Bytes) / float64(maxBytes) * 100)
		}
		if e.IsDir {
			row.BrowseHref = browseHref(id, e.Path)
		}
		view.Entries = append(view.Entries, row)
	}
	s.render(w, r, templates.BrowseFragment(view))
}

// humanScannedAt turns the stored scan timestamp into a relative phrase
// ("hace 3 min"). SQLite writes datetime('now') in UTC, so showing the raw
// value would be hours off from the user's clock.
func humanScannedAt(stored string) string {
	t, err := time.Parse(time.DateTime, stored)
	if err != nil {
		return stored
	}
	switch d := time.Since(t.UTC()); {
	case d < 0:
		return "recién"
	case d < time.Minute:
		return "hace instantes"
	case d < time.Hour:
		return fmt.Sprintf("hace %d min", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("hace %d h", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("hace %d días", int(d.Hours()/24))
	default:
		return t.UTC().Local().Format("2006-01-02 15:04")
	}
}

func browseHref(folderID int64, path string) string {
	return "/disk/browse?folder=" + strconv.FormatInt(folderID, 10) + "&path=" + url.QueryEscape(path)
}
