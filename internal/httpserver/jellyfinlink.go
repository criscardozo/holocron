package httpserver

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/cristian/holocron/internal/jellyfin"
	"github.com/cristian/holocron/internal/settings"
	"github.com/cristian/holocron/web/templates"
)

// Quick Connect: Holocron shows a code, the user approves it in Jellyfin, and
// the token arrives without anyone copying an API key. The UI polls the status
// endpoint while a code is outstanding.

func (s *Server) handleJellyfinLinkStart(w http.ResponseWriter, r *http.Request) {
	status, err := s.deps.JellyfinLink.Start(r.Context())
	if err != nil {
		s.log.Warn("jellyfin link start", "error", err)
		s.render(w, r, templates.JellyfinLink(templates.JellyfinLinkView{
			Error: linkErrorMessage(err),
		}))
		return
	}
	s.render(w, r, templates.JellyfinLink(linkView(status)))
}

func (s *Server) handleJellyfinLinkStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.deps.JellyfinLink.Check(r.Context())
	switch {
	case errors.Is(err, jellyfin.ErrNoLinkInProgress):
		s.render(w, r, templates.JellyfinLink(templates.JellyfinLinkView{}))
	case err != nil:
		s.log.Warn("jellyfin link check", "error", err)
		s.render(w, r, templates.JellyfinLink(templates.JellyfinLinkView{
			Error: linkErrorMessage(err),
		}))
	default:
		s.render(w, r, templates.JellyfinLink(linkView(status)))
	}
}

func (s *Server) handleJellyfinLinkCancel(w http.ResponseWriter, r *http.Request) {
	s.deps.JellyfinLink.Cancel()
	s.redirect(w, r, "/settings")
}

func (s *Server) handleJellyfinUnlink(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.JellyfinLink.Unlink(r.Context()); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, "/settings?notice="+url.QueryEscape("Jellyfin desvinculado."))
}

// handleSaveJellyfinURL stores the server address, which has to come before the
// code: unlike Plex there is no cloud service to discover the server through.
func (s *Server) handleSaveJellyfinURL(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if err := s.deps.Settings.Set(r.Context(), settings.KeyJellyfinURL,
		strings.TrimSpace(r.PostFormValue("url"))); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, "/settings")
}

func (s *Server) handleJellyfinTest(w http.ResponseWriter, r *http.Request) {
	info, err := s.deps.Library.TestConnection(r.Context())
	if err != nil {
		s.log.Warn("jellyfin test", "error", err)
		s.render(w, r, templates.JellyfinTest(false,
			"No se pudo conectar con Jellyfin. Revisá la dirección y la vinculación."))
		return
	}
	name := info.Name
	if name == "" {
		name = "Jellyfin"
	}
	s.render(w, r, templates.JellyfinTest(true, "Conectado a "+name+" "+info.Version))
}

// linkErrorMessage keeps server detail out of the page while still telling the
// user which of the few actionable problems they have.
func linkErrorMessage(err error) string {
	switch {
	case errors.Is(err, jellyfin.ErrNoServerURL):
		return "Cargá primero la dirección de Jellyfin."
	case errors.Is(err, jellyfin.ErrQuickConnectDisabled):
		return "Quick Connect está desactivado en Jellyfin. Activalo en el panel del servidor (Dashboard → General)."
	default:
		return "No se pudo hablar con Jellyfin. Probá de nuevo."
	}
}

// linkView maps the service status onto the fragment's view model.
func linkView(status jellyfin.Status) templates.JellyfinLinkView {
	v := templates.JellyfinLinkView{Code: status.Code, User: status.User, Admin: status.Admin}
	switch status.State {
	case jellyfin.StatePending:
		v.Pending = true
	case jellyfin.StateLinked:
		v.Linked = true
	case jellyfin.StateExpired:
		v.Error = "El código venció. Generá uno nuevo."
	case jellyfin.StateIdle:
		// Nothing in progress: the fragment renders the start button.
	}
	return v
}
