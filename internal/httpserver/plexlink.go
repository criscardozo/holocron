package httpserver

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/cristian/holocron/internal/plexauth"
	"github.com/cristian/holocron/web/templates"
)

// The device-link flow lets the user authorise Holocron at plex.tv instead of
// digging an X-Plex-Token out of the browser. The UI shows a code, then polls
// this endpoint until plex.tv reports the PIN as authorised.

func (s *Server) handlePlexLinkStart(w http.ResponseWriter, r *http.Request) {
	status, err := s.deps.PlexAuth.Start(r.Context())
	if err != nil {
		s.log.Warn("plex link start", "error", err)
		s.render(w, r, templates.PlexLink(templates.PlexLinkView{
			Error: "No se pudo contactar a plex.tv. Revisá la conexión a internet.",
		}))
		return
	}
	s.render(w, r, templates.PlexLink(linkView(status)))
}

func (s *Server) handlePlexLinkStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.deps.PlexAuth.Check(r.Context())
	switch {
	case errors.Is(err, plexauth.ErrNoLinkInProgress):
		s.render(w, r, templates.PlexLink(templates.PlexLinkView{}))
		return
	case err != nil:
		s.log.Warn("plex link check", "error", err)
		s.render(w, r, templates.PlexLink(templates.PlexLinkView{
			Error: "Se perdió la conexión con plex.tv. Probá de nuevo.",
		}))
		return
	}
	s.render(w, r, templates.PlexLink(linkView(status)))
}

func (s *Server) handlePlexLinkServer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if err := s.deps.PlexAuth.SelectServer(r.Context(), r.PostFormValue("base_url")); err != nil {
		s.log.Warn("plex select server", "error", err)
		s.redirect(w, r, "/settings?notice="+url.QueryEscape("No se pudo guardar el servidor."))
		return
	}
	s.redirect(w, r, "/settings?notice="+url.QueryEscape("Plex conectado."))
}

func (s *Server) handlePlexLinkCancel(w http.ResponseWriter, r *http.Request) {
	s.deps.PlexAuth.Cancel()
	s.redirect(w, r, "/settings")
}

// linkView maps the service status onto the fragment's view model.
func linkView(status plexauth.Status) templates.PlexLinkView {
	v := templates.PlexLinkView{
		Code:    status.Code,
		AuthURL: status.AuthURL,
	}
	switch status.State {
	case plexauth.StatePending:
		v.Pending = true
	case plexauth.StateLinked:
		v.Linked = true
		for _, srv := range status.Servers {
			v.Servers = append(v.Servers, templates.PlexServerOption{
				Name:    srv.Name,
				BaseURL: srv.BaseURL,
			})
		}
	case plexauth.StateExpired:
		v.Error = "El código venció. Generá uno nuevo."
	case plexauth.StateIdle:
		// Nothing in progress: the fragment renders the start button.
	}
	return v
}
