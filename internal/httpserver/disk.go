package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/scanner"
	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/web/templates"
)

func (s *Server) handleDiskPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.deps.Folders.List(ctx, folders.PurposeDisk)
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	view := templates.DiskPageView{}
	if len(list) == 0 {
		view.Empty = true
		s.render(w, r, templates.DiskPage(view))
		return
	}

	selID := list[0].ID
	if q := r.URL.Query().Get("folder"); q != "" {
		if id, err := strconv.ParseInt(q, 10, 64); err == nil && containsFolder(list, id) {
			selID = id
		}
	}

	var selected folders.Folder
	for _, f := range list {
		active := f.ID == selID
		if active {
			selected = f
		}
		view.Nav = append(view.Nav, templates.DiskNavItem{
			Label:  f.Label,
			Href:   "/disk?folder=" + strconv.FormatInt(f.ID, 10),
			Active: active,
		})
	}

	detail := s.buildDiskDetail(ctx, selected)
	view.Selected = &detail
	s.render(w, r, templates.DiskPage(view))
}

func (s *Server) buildDiskDetail(ctx context.Context, f folders.Folder) templates.DiskDetail {
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
		d.ScannedAt = scannedAt
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
		for _, e := range res.Errors {
			d.Errors = append(d.Errors, e.Path+": "+e.Message)
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
	for _, e := range res.Entries {
		row := templates.BrowseRow{
			Name:       e.Name,
			BytesHuman: system.HumanBytes(e.Bytes),
			IsDir:      e.IsDir,
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

func browseHref(folderID int64, path string) string {
	return "/disk/browse?folder=" + strconv.FormatInt(folderID, 10) + "&path=" + url.QueryEscape(path)
}

func containsFolder(list []folders.Folder, id int64) bool {
	for _, f := range list {
		if f.ID == id {
			return true
		}
	}
	return false
}
