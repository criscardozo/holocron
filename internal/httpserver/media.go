package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/library"
	"github.com/cristian/holocron/web/templates"
)

func (s *Server) handleMediaPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.MediaPage(s.mediaView(r.Context())))
}

func (s *Server) mediaView(ctx context.Context) templates.MediaPageView {
	v := templates.MediaPageView{Configured: s.deps.Library.Configured(ctx)}
	if !v.Configured {
		return v
	}
	v.Syncing = s.deps.Library.Syncing()
	if st, err := s.deps.Library.Stats(ctx); err == nil {
		v.Total, v.Movies, v.WithoutSubs = st.Total, st.Movies, st.WithoutSubs
	}
	const listLimit = 500
	if items, err := s.deps.Library.Items(ctx, listLimit); err == nil {
		for _, it := range items {
			v.Items = append(v.Items, templates.MediaItemRow{
				Title:     it.Title,
				Year:      it.Year,
				Type:      it.Type,
				Path:      it.Path,
				HasSubsES: it.HasSubsES,
			})
		}
		v.Truncated = len(items) >= listLimit && v.Total > len(items)
	}
	return v
}

func (s *Server) handleMediaSync(w http.ResponseWriter, r *http.Request) {
	_, err := s.deps.Library.StartSync(r.Context())
	s.renderJobStart(w, r, err, "Sincronizando…", "/media/status")
}

func (s *Server) renderJobStart(w http.ResponseWriter, r *http.Request, err error, label, statusHref string) {
	if err != nil && !errors.Is(err, jobs.ErrKindBusy) {
		msg := "No se pudo iniciar."
		if errors.Is(err, library.ErrNotConfigured) {
			msg = "Jellyfin no está vinculado."
		}
		s.log.Warn("start media job", "error", err)
		s.render(w, r, templates.JobStatus(templates.JobStatusView{Error: msg}))
		return
	}
	s.render(w, r, templates.JobStatus(templates.JobStatusView{
		Running:    true,
		Label:      label,
		StatusHref: statusHref,
	}))
}

func (s *Server) handleMediaStatus(w http.ResponseWriter, r *http.Request) {
	const label, statusHref = "Sincronizando…", "/media/status"
	if s.deps.Library.Syncing() {
		s.render(w, r, templates.JobStatus(templates.JobStatusView{
			Running: true, Label: label, StatusHref: statusHref,
		}))
		return
	}

	view := templates.JobStatusView{
		ReloadHref:   "/media",
		ReloadSelect: "#media-detail",
		ReloadTarget: "#media-detail",
	}
	if job, ok := s.deps.Library.LastJob(); ok {
		if job.Status == jobs.StatusError {
			view.Error = jobFailureMessage(job, "El trabajo falló.")
		} else {
			view.Summary = job.Result
		}
	}
	s.render(w, r, templates.JobStatus(view))
}
