package httpserver

import (
	"errors"
	"net/http"

	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/web/templates"
)

// The bulk rename screen. Nothing here acts without a preview having been on
// screen first; see web/templates/rename.templ for why that is the feature
// rather than a formality.

func (s *Server) handleRenamePage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.RenamePage(s.renameView(r, templates.RenamePageView{})))
}

func (s *Server) handleRenamePreview(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Naming.StartPreview(r.Context()); err != nil && !errors.Is(err, jobs.ErrKindBusy) {
		s.log.Warn("start rename preview", "error", err)
		s.render(w, r, templates.RenameSection(s.renameView(r, templates.RenamePageView{
			Notice: "No se pudo revisar las carpetas.", NoticeErr: true,
		})))
		return
	}
	s.render(w, r, templates.RenameSection(s.renameView(r, templates.RenamePageView{})))
}

func (s *Server) handleRenameStatus(w http.ResponseWriter, r *http.Request) {
	view := templates.RenamePageView{}
	if !s.deps.Naming.Previewing() && !s.deps.Naming.Renaming() {
		// Report the outcome of whichever one just finished. Apply is checked
		// first: after a rename its summary is the news, and the preview that
		// follows it is only bookkeeping.
		if job, ok := s.deps.Naming.LastApplyJob(); ok && job.Status == jobs.StatusError {
			view.Notice, view.NoticeErr = jobFailureMessage(job, "El renombrado falló."), true
		} else if ok && job.Result != "" {
			view.Notice = job.Result
		} else if job, ok := s.deps.Naming.LastPreviewJob(); ok && job.Status == jobs.StatusError {
			view.Notice, view.NoticeErr = jobFailureMessage(job, "No se pudo revisar."), true
		}
	}
	s.render(w, r, templates.RenameSection(s.renameView(r, view)))
}

func (s *Server) handleRenameApply(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	keys := r.PostForm["folder"]
	if len(keys) == 0 {
		s.render(w, r, templates.RenameSection(s.renameView(r, templates.RenamePageView{
			Notice: "No tildaste ninguna carpeta.", NoticeErr: true,
		})))
		return
	}
	if err := s.deps.Naming.StartApply(r.Context(), keys); err != nil && !errors.Is(err, jobs.ErrKindBusy) {
		s.log.Warn("start rename", "error", err, "folders", len(keys))
		s.render(w, r, templates.RenameSection(s.renameView(r, templates.RenamePageView{
			Notice: "No se pudo renombrar.", NoticeErr: true,
		})))
		return
	}
	s.log.Info("bulk rename requested", "folders", len(keys))
	s.render(w, r, templates.RenameSection(s.renameView(r, templates.RenamePageView{})))
}

// renameView assembles the screen from the last preview.
func (s *Server) renameView(r *http.Request, base templates.RenamePageView) templates.RenamePageView {
	ctx := r.Context()
	v := base
	v.HasMediaFolders = s.deps.Naming.HasMediaFolders(ctx)
	v.Running = s.deps.Naming.Previewing() || s.deps.Naming.Renaming()

	_, previewed := s.deps.Naming.LastPreviewJob()
	v.Previewed = previewed

	for _, pf := range s.deps.Naming.Plans() {
		if pf.Plan.Blocked != "" {
			v.Blocked = append(v.Blocked, templates.RenameBlockedRow{
				Label: pf.Label, Folder: pf.Plan.Folder, Reason: pf.Plan.Blocked,
			})
			continue
		}
		row := templates.RenamePlanRow{
			Key:   pf.Key(),
			Label: pf.Label,
			From:  pf.Plan.Folder,
			To:    pf.Plan.NewFolder,
		}
		for _, f := range pf.Plan.Files {
			row.Files = append(row.Files, templates.RenameFileRow{From: f.From, To: f.To})
		}
		for _, sk := range pf.Plan.Skipped {
			row.Skipped = append(row.Skipped, sk.Name+" — "+sk.Reason)
		}
		v.TotalFiles += len(row.Files)
		v.Plans = append(v.Plans, row)
	}
	return v
}
