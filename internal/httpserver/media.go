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
	v.GeneratingNFO = s.deps.Library.GeneratingNFO()
	if st, err := s.deps.Library.Stats(ctx); err == nil {
		v.Total, v.WithNFO, v.WithoutSubs = st.Total, st.WithNFO, st.WithoutSubs
	}
	if items, err := s.deps.Library.Items(ctx, 500); err == nil {
		for _, it := range items {
			v.Items = append(v.Items, templates.MediaItemRow{
				Title:     it.Title,
				Year:      it.Year,
				Type:      it.Type,
				HasNFO:    it.HasNFO,
				HasSubsES: it.HasSubsES,
			})
		}
	}
	return v
}

func (s *Server) handleMediaSync(w http.ResponseWriter, r *http.Request) {
	_, err := s.deps.Library.StartSync(r.Context())
	s.renderJobStart(w, r, err, "Sincronizando…", "/media/status?job=sync")
}

func (s *Server) handleMediaNFO(w http.ResponseWriter, r *http.Request) {
	_, err := s.deps.Library.StartGenerateNFO(r.Context())
	s.renderJobStart(w, r, err, "Generando .nfo…", "/media/status?job=nfo")
}

func (s *Server) renderJobStart(w http.ResponseWriter, r *http.Request, err error, label, statusHref string) {
	if err != nil && !errors.Is(err, jobs.ErrKindBusy) {
		msg := "No se pudo iniciar."
		if errors.Is(err, library.ErrNotConfigured) {
			msg = "Plex no está configurado."
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
	kind := library.KindSync
	label, statusHref := "Sincronizando…", "/media/status?job=sync"
	running := s.deps.Library.Syncing()
	if r.URL.Query().Get("job") == "nfo" {
		kind = library.KindNFO
		label, statusHref = "Generando .nfo…", "/media/status?job=nfo"
		running = s.deps.Library.GeneratingNFO()
	}

	if running {
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
	if job, ok := s.deps.Library.LastJob(kind); ok {
		if job.Status == jobs.StatusError {
			view.Error = "El trabajo falló."
		} else {
			view.Summary = job.Result
		}
	}
	s.render(w, r, templates.JobStatus(view))
}
