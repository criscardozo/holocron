package httpserver

import (
	"context"
	"net/http"

	"github.com/cristian/holocron/web/templates"
)

// The update panel checks GitHub for a newer release and, when the privileged
// helper is installed, asks it to install one. Holocron cannot replace its own
// binary: see internal/updates.

func (s *Server) handleUpdatesCheck(w http.ResponseWriter, r *http.Request) {
	// A button press is an explicit request, so bypass the cache.
	s.render(w, r, templates.Updates(s.updatesView(r.Context(), true)))
}

func (s *Server) handleUpdatesInstall(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Updates.RequestInstall(); err != nil {
		s.log.Warn("request update", "error", err)
		view := s.updatesView(r.Context(), false)
		view.Error = "No se pudo pedir la actualización. ¿Está instalado el helper?"
		s.render(w, r, templates.Updates(view))
		return
	}
	s.render(w, r, templates.Updates(templates.UpdatesView{
		Requested: "Descargando e instalando la versión nueva…",
	}))
}

// updatesView maps the service status onto the panel's view model. With
// fetch=false nothing is requested from GitHub, so the settings page renders
// instantly and works offline.
func (s *Server) updatesView(ctx context.Context, fetch bool) templates.UpdatesView {
	st := s.deps.Updates.Cached()
	if fetch {
		st = s.deps.Updates.Status(ctx, true)
	}
	v := templates.UpdatesView{
		Current:     st.Current,
		Latest:      st.Latest,
		Notes:       st.Notes,
		URL:         st.URL,
		Available:   st.Available,
		Checkable:   st.Checkable,
		Installable: st.Installable,
		Error:       st.Error,
	}
	if !st.CheckedAt.IsZero() {
		v.CheckedAt = "a las " + st.CheckedAt.Local().Format("15:04")
	}
	return v
}
