package httpserver

import (
	"net/http"

	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/web/templates"
)

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Folders.List(r.Context(), "")
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	view := templates.SettingsView{
		Purposes: []string{folders.PurposeDisk, folders.PurposeMovies, folders.PurposeTV},
		Notice:   r.URL.Query().Get("notice"),
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
	if err != nil {
		s.log.Warn("add folder", "error", err)
		s.redirect(w, r, "/settings?notice=No+se+pudo+agregar+la+carpeta")
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
