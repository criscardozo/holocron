package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/quality"
	"github.com/cristian/holocron/web/templates"
)

// The quality panel is the one screen that reports what is wrong with the
// library rather than how big it is. It renders the last cached audit; running
// a new one is an explicit button, because the audit asks Jellyfin for the
// whole library including episodes.

func (s *Server) handleQualityPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.QualityPage(s.qualityView(r.Context(), r.URL.Query().Get("cat"))))
}

func (s *Server) handleQualityScan(w http.ResponseWriter, r *http.Request) {
	_, err := s.deps.Quality.StartScan(r.Context())
	if err != nil && !errors.Is(err, jobs.ErrKindBusy) {
		msg := "No se pudo iniciar el análisis."
		if errors.Is(err, quality.ErrNotConfigured) {
			msg = "Jellyfin no está vinculado."
		}
		s.log.Warn("start quality scan", "error", err)
		s.render(w, r, templates.JobStatus(templates.JobStatusView{Error: msg}))
		return
	}
	s.render(w, r, templates.JobStatus(templates.JobStatusView{
		Running:    true,
		Label:      "Analizando la biblioteca…",
		StatusHref: "/quality/status",
	}))
}

func (s *Server) handleQualityStatus(w http.ResponseWriter, r *http.Request) {
	if s.deps.Quality.Scanning() {
		s.render(w, r, templates.JobStatus(templates.JobStatusView{
			Running:    true,
			Label:      "Analizando la biblioteca…",
			StatusHref: "/quality/status",
		}))
		return
	}

	view := templates.JobStatusView{
		ReloadHref:   "/quality",
		ReloadSelect: "#quality-detail",
		ReloadTarget: "#quality-detail",
	}
	if job, ok := s.deps.Quality.LastJob(); ok {
		if job.Status == jobs.StatusError {
			view.Error = "El análisis falló."
		} else {
			view.Summary = job.Result
		}
	}
	s.render(w, r, templates.JobStatus(view))
}

// handleQualityRefresh asks Jellyfin to re-read one item's metadata. It answers
// with a fragment that replaces the row's action cell: the outcome belongs next
// to the row it is about, and re-rendering the whole table would lose the
// user's place in a list of two hundred.
func (s *Server) handleQualityRefresh(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	err := s.deps.Quality.Refresh(r.Context(), r.PostFormValue("item"))
	view := templates.QualityRefreshView{Message: "Pedido"}
	switch {
	case err == nil:
	case errors.Is(err, quality.ErrNotAdmin):
		view = templates.QualityRefreshView{Message: "Requiere admin", Error: true}
	case errors.Is(err, quality.ErrUnknownItem), errors.Is(err, quality.ErrNoReport):
		view = templates.QualityRefreshView{Message: "Volvé a analizar", Error: true}
	default:
		s.log.Warn("quality refresh", "error", err)
		view = templates.QualityRefreshView{Message: "Falló", Error: true}
	}
	s.render(w, r, templates.QualityRefreshResult(view))
}

// qualityView maps the cached report onto the page. selected is the category
// whose list is shown; an unknown or empty value falls back to the first one
// that has findings, so the panel opens on something rather than on an empty
// table.
func (s *Server) qualityView(ctx context.Context, selected string) templates.QualityPageView {
	v := templates.QualityPageView{Configured: s.deps.Quality.Configured(ctx)}
	if !v.Configured {
		return v
	}
	v.Scanning = s.deps.Quality.Scanning()
	v.Admin = s.deps.Quality.Admin(ctx)

	report, ok, err := s.deps.Quality.Latest(ctx)
	if err != nil {
		s.log.Warn("read quality report", "error", err)
		return v
	}
	if !ok {
		return v
	}

	v.HasReport = true
	v.Scanned = report.Scanned
	v.Total = report.Total()
	if !report.GeneratedAt.IsZero() {
		v.GeneratedAt = report.GeneratedAt.Local().Format("02/01 15:04")
	}

	active := pickCategory(report, selected)
	for _, c := range quality.Categories {
		v.Counts = append(v.Counts, templates.QualityCount{
			Key:    string(c),
			Label:  c.Label(),
			Count:  report.Count(c),
			Href:   "/quality?cat=" + url.QueryEscape(string(c)),
			Active: c == active,
		})
	}

	group := templates.QualityGroupView{
		Key:         string(active),
		Label:       active.Label(),
		Hint:        active.Hint(),
		Count:       report.Count(active),
		Refreshable: active.Refreshable(),
	}
	rows := report.For(active)
	group.Truncated = len(rows) < group.Count
	for _, f := range rows {
		group.Rows = append(group.Rows, templates.QualityRow{
			ItemID: f.ItemID,
			Title:  f.Title,
			Detail: f.Detail,
			Path:   f.Path,
			Kind:   f.Kind,
		})
	}
	v.Selected = &group
	return v
}

// pickCategory resolves the requested category, defaulting to the first one
// with findings.
func pickCategory(report quality.Report, requested string) quality.Category {
	requested = strings.TrimSpace(requested)
	for _, c := range quality.Categories {
		if string(c) == requested {
			return c
		}
	}
	for _, c := range quality.Categories {
		if report.Count(c) > 0 {
			return c
		}
	}
	return quality.Categories[0]
}
