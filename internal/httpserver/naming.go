package httpserver

import (
	"net/http"

	"github.com/cristian/holocron/web/templates"
)

func (s *Server) handleNamingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	view := templates.NamingPageView{HasMediaFolders: s.deps.Naming.HasMediaFolders(ctx)}
	issues, err := s.deps.Naming.Issues(ctx)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	view.Count = len(issues)
	for _, is := range issues {
		view.Issues = append(view.Issues, templates.NamingIssueRow{
			Type:     is.Type,
			Found:    is.Found,
			Expected: is.Expected,
			Path:     is.Path,
		})
	}
	s.render(w, r, templates.NamingPage(view))
}

func (s *Server) handleNamingScan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, err := s.deps.Naming.Scan(ctx); err != nil {
		s.log.Warn("naming scan", "error", err)
	}
	if r.URL.Query().Get("to") == "page" {
		s.redirect(w, r, "/naming")
		return
	}
	view := templates.NamingCardView{HasMediaFolders: s.deps.Naming.HasMediaFolders(ctx)}
	if count, err := s.deps.Naming.Count(ctx); err == nil {
		view.Count = count
	}
	s.render(w, r, templates.NamingCard(view))
}
