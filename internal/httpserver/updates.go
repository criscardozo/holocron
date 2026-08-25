package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/cristian/holocron/internal/apitoken"
	"github.com/cristian/holocron/web/templates"
)

// The update panel checks GitHub for a newer release and, when the privileged
// helper is installed, asks it to install one. Holocron cannot replace its own
// binary: see internal/updates.

func (s *Server) handleUpdatesCheck(w http.ResponseWriter, r *http.Request) {
	// A button press is an explicit request, so bypass the cache.
	s.render(w, r, templates.Updates(s.updatesView(r.Context(), true)))
}

// handleUpdatesInstall requires the API token. The web UI is otherwise
// unauthenticated by design (trusted LAN), but replacing the binary and
// restarting the service is the one action here that should not be available to
// whoever happens to be on the network. The token already exists for the app,
// so this adds a gate without adding a second credential to manage.
func (s *Server) handleUpdatesInstall(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	fail := func(message string) {
		view := s.updatesView(ctx, false)
		view.Error = message
		s.render(w, r, templates.Updates(view))
	}

	token := strings.TrimSpace(r.PostFormValue("token"))
	if token == "" {
		fail("Pegá el token de la API para confirmar la actualización.")
		return
	}
	if err := s.deps.APIToken.Verify(ctx, token); err != nil {
		if errors.Is(err, apitoken.ErrNoToken) {
			fail("No hay token generado. Generá uno en «App iOS», más abajo, y pegalo acá.")
			return
		}
		// Deliberately vague and logged as a warning: a wrong token here is
		// either a typo or someone probing.
		s.log.Warn("rejected update request", "reason", "invalid token")
		fail("El token no coincide.")
		return
	}

	if err := s.deps.Updates.RequestInstall(); err != nil {
		s.log.Warn("request update", "error", err)
		fail("No se pudo pedir la actualización. ¿Está instalado el helper?")
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
