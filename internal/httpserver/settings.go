package httpserver

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/settings"
	"github.com/cristian/holocron/web/templates"
)

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.deps.Folders.List(ctx, "")
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	view := templates.SettingsView{
		Purposes:    []string{folders.PurposeDisk, folders.PurposeMovies, folders.PurposeTV},
		Notice:      r.URL.Query().Get("notice"),
		JellyfinURL: s.deps.Settings.GetDefault(ctx, settings.KeyJellyfinURL, ""),
	}
	view.OpenSubsUser = s.deps.Settings.GetDefault(ctx, settings.KeyOpenSubtitlesUser, "")
	if _, ok, _ := s.deps.Settings.Get(ctx, settings.KeyOpenSubtitlesKey); ok {
		view.OpenSubsSet = true
	}
	view.QbitURL = s.deps.Settings.GetDefault(ctx, settings.KeyQbitURL, "")
	view.QbitUser = s.deps.Settings.GetDefault(ctx, settings.KeyQbitUser, "")
	if _, ok, _ := s.deps.Settings.Get(ctx, settings.KeyQbitPass); ok {
		view.QbitSet = true
	}
	view.APITokenSet = s.deps.APIToken.Configured(ctx)
	view.Updates = s.updatesView(ctx, false)
	// A reload mid-flow should keep showing the code rather than restart it.
	if s.deps.JellyfinLink.Pending() {
		if status, err := s.deps.JellyfinLink.Check(ctx); err == nil {
			view.JellyfinLink = linkView(status)
		}
	}
	for _, f := range list {
		view.Folders = append(view.Folders, templates.SettingsFolderRow{
			ID:      f.ID,
			Label:   f.Label,
			Path:    f.Path,
			Purpose: f.Purpose,
		})
	}
	s.render(w, r, templates.SettingsPage(view))
}

func (s *Server) handleAddFolder(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	_, err := s.deps.Folders.Add(r.Context(),
		r.PostFormValue("label"), r.PostFormValue("path"), r.PostFormValue("purpose"))
	switch {
	case errors.Is(err, folders.ErrNotADirectory):
		s.log.Warn("add folder", "error", err)
		s.redirect(w, r, "/settings?notice="+url.QueryEscape("Esa ruta no existe o no es una carpeta."))
		return
	case err != nil:
		s.log.Warn("add folder", "error", err)
		s.redirect(w, r, "/settings?notice="+url.QueryEscape("No se pudo agregar la carpeta."))
		return
	}
	s.redirect(w, r, "/settings")
}

func (s *Server) handleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := s.formInt64(w, r, "id")
	if !ok {
		return
	}
	if err := s.deps.Folders.Delete(r.Context(), id); err != nil {
		s.log.Warn("delete folder", "id", id, "error", err)
	}
	s.redirect(w, r, "/settings")
}

func (s *Server) handleSaveOpenSubtitles(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	// Username is always stored (may be blank); secrets only when provided.
	if err := s.deps.Settings.Set(ctx, settings.KeyOpenSubtitlesUser, strings.TrimSpace(r.PostFormValue("username"))); err != nil {
		s.serverError(w, r, err)
		return
	}
	if key := strings.TrimSpace(r.PostFormValue("api_key")); key != "" {
		if err := s.deps.Settings.Set(ctx, settings.KeyOpenSubtitlesKey, key); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	if pass := r.PostFormValue("password"); pass != "" {
		if err := s.deps.Settings.Set(ctx, settings.KeyOpenSubtitlesPass, pass); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	s.redirect(w, r, "/settings")
}

func (s *Server) handleSaveQbit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if err := s.deps.Settings.Set(ctx, settings.KeyQbitURL, strings.TrimSpace(r.PostFormValue("url"))); err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.deps.Settings.Set(ctx, settings.KeyQbitUser, strings.TrimSpace(r.PostFormValue("username"))); err != nil {
		s.serverError(w, r, err)
		return
	}
	if pass := r.PostFormValue("password"); pass != "" {
		if err := s.deps.Settings.Set(ctx, settings.KeyQbitPass, pass); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	s.redirect(w, r, "/settings")
}

// handleGenerateAPIToken issues a new API token and shows it once. Only its
// digest is stored, so this is the only chance to copy it.
func (s *Server) handleGenerateAPIToken(w http.ResponseWriter, r *http.Request) {
	token, err := s.deps.APIToken.Generate(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.APITokenIssued(token))
}

func (s *Server) handleRevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.APIToken.Revoke(r.Context()); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, "/settings?notice="+url.QueryEscape("Token de la API revocado."))
}
