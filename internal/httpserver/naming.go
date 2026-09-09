package httpserver

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

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
	view.Notice = r.URL.Query().Get("notice")
	if paths, err := s.deps.Naming.IgnoredPaths(ctx); err == nil {
		for _, p := range paths {
			view.Ignored = append(view.Ignored, templates.NamingIgnoredRow{
				Path: p, Name: filepath.Base(p),
			})
		}
	}
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

// handleNamingIgnore and handleNamingUnignore let the user take a folder off
// the list, and put it back.
//
// Both take an absolute path straight from the form, and neither validates it
// against anything. That is safe because of what the list is: a set of strings
// that scanning compares folders against. Nothing is opened, read or written
// from it, so an invented path adds a row that will never match. The check
// that matters — that a rename stays inside a configured media folder — is
// still done where the rename happens.
func (s *Server) handleNamingIgnore(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.PostFormValue("path"))
	if path == "" {
		s.redirect(w, r, "/naming")
		return
	}
	if err := s.deps.Naming.Ignore(r.Context(), path); err != nil {
		s.serverError(w, r, err)
		return
	}
	// Re-scan so the row disappears now rather than at the next refresh.
	if _, err := s.deps.Naming.Scan(r.Context()); err != nil {
		s.log.Warn("rescan after ignore", "error", err)
	}
	s.log.Info("folder ignored", "path", path)
	s.redirect(w, r, "/naming?notice="+url.QueryEscape("Listo, no se avisa más sobre «"+filepath.Base(path)+"»."))
}

func (s *Server) handleNamingUnignore(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.PostFormValue("path"))
	if path == "" {
		s.redirect(w, r, "/naming")
		return
	}
	if err := s.deps.Naming.Unignore(r.Context(), path); err != nil {
		s.serverError(w, r, err)
		return
	}
	if _, err := s.deps.Naming.Scan(r.Context()); err != nil {
		s.log.Warn("rescan after unignore", "error", err)
	}
	s.redirect(w, r, "/naming?notice="+url.QueryEscape("«"+filepath.Base(path)+"» vuelve a revisarse."))
}
