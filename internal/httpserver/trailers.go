package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/trailers"
	"github.com/cristian/holocron/web/templates"
)

// The trailers screen. Everything here depends on an external tool that may be
// missing or out of date, so the page's job is as much to explain the state of
// yt-dlp as it is to list films.

func (s *Server) handleTrailersPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.TrailersPage(s.trailersView(r, templates.TrailersPageView{})))
}

func (s *Server) handleTrailersScan(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Trailers.StartScan(r.Context()); err != nil && !errors.Is(err, jobs.ErrKindBusy) {
		s.log.Warn("start trailer scan", "error", err)
		s.render(w, r, templates.TrailersSection(s.trailersView(r, templates.TrailersPageView{
			Notice: "No se pudo revisar las carpetas.", NoticeErr: true,
		})))
		return
	}
	s.render(w, r, templates.TrailersSection(s.trailersView(r, templates.TrailersPageView{})))
}

func (s *Server) handleTrailersStatus(w http.ResponseWriter, r *http.Request) {
	view := templates.TrailersPageView{}
	if !s.deps.Trailers.Scanning() && !s.deps.Trailers.Fetching() {
		if job, ok := s.deps.Trailers.LastFetchJob(); ok && job.Status == jobs.StatusError {
			view.Notice, view.NoticeErr = jobFailureMessage(job, "La descarga falló."), true
		} else if ok && job.Result != "" {
			view.Notice = job.Result
		} else if job, ok := s.deps.Trailers.LastScanJob(); ok {
			if job.Status == jobs.StatusError {
				view.Notice, view.NoticeErr = jobFailureMessage(job, "No se pudo revisar."), true
			} else {
				view.Notice = job.Result
			}
		}
	}
	s.render(w, r, templates.TrailersSection(s.trailersView(r, view)))
}

func (s *Server) handleTrailersFetch(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	keys := r.PostForm["film"]
	if len(keys) == 0 {
		s.render(w, r, templates.TrailersSection(s.trailersView(r, templates.TrailersPageView{
			Notice: "No tildaste ninguna película.", NoticeErr: true,
		})))
		return
	}
	err := s.deps.Trailers.StartFetch(r.Context(), keys)
	switch {
	case err == nil, errors.Is(err, jobs.ErrKindBusy):
		s.log.Info("trailer fetch requested", "films", len(keys))
		s.render(w, r, templates.TrailersSection(s.trailersView(r, templates.TrailersPageView{})))
	case errors.Is(err, trailers.ErrNoYTDLP):
		s.render(w, r, templates.TrailersSection(s.trailersView(r, templates.TrailersPageView{
			Notice: "yt-dlp no está instalado en la Pi, así que no se puede bajar nada.", NoticeErr: true,
		})))
	default:
		s.log.Warn("start trailer fetch", "error", err)
		s.render(w, r, templates.TrailersSection(s.trailersView(r, templates.TrailersPageView{
			Notice: "No se pudo empezar la descarga.", NoticeErr: true,
		})))
	}
}

func (s *Server) trailersView(r *http.Request, base templates.TrailersPageView) templates.TrailersPageView {
	ctx := r.Context()
	v := base
	v.HasMediaFolders = s.deps.Naming.HasMediaFolders(ctx)
	v.Running = s.deps.Trailers.Scanning() || s.deps.Trailers.Fetching()
	v.ToolPath, v.ToolVersion, v.ToolStale = s.deps.Trailers.ToolInfo(ctx)

	_, v.Scanned = s.deps.Trailers.LastScanJob()

	for _, f := range s.deps.Trailers.Missing() {
		row := templates.TrailerFilmRow{
			Key: f.Dir, Folder: f.Folder, Title: f.Title, NoYear: f.Year == 0,
		}
		if f.Year > 0 {
			row.Year = strconv.Itoa(f.Year)
		}
		v.Films = append(v.Films, row)
	}
	for _, res := range s.deps.Trailers.Results() {
		v.Results = append(v.Results, templates.TrailerResultRow{
			Folder: res.Folder, Trailer: res.Trailer, Reason: res.Reason, Err: res.Err,
		})
	}
	return v
}
