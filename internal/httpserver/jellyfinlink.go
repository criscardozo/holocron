package httpserver

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/cristian/holocron/internal/jellyfin"
	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/netaddr"
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
//
// The address is normalised rather than stored as typed: "192.168.0.2:8096" is
// what anyone writes down and is not a URL, and storing it that way makes every
// later call fail with a generic "could not talk to Jellyfin".
func (s *Server) handleSaveJellyfinURL(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	address, err := netaddr.Normalise(r.PostFormValue("url"))
	if err != nil {
		s.log.Warn("save jellyfin url", "error", err)
		s.redirect(w, r, "/settings?notice="+url.QueryEscape(
			"Esa dirección no se entiende. Va algo como 192.168.0.2:8096 o http://obiwan:8096."))
		return
	}
	if err := s.deps.Settings.Set(r.Context(), settings.KeyJellyfinURL, address); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, "/settings")
}

// handleJellyfinTest answers two different questions depending on how far the
// setup has got, because "the address is wrong" and "you have not linked yet"
// used to produce the same failure and are fixed in different places.
func (s *Server) handleJellyfinTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if s.deps.Library.Configured(ctx) {
		info, err := s.deps.Library.TestConnection(ctx)
		if err != nil {
			s.log.Warn("jellyfin test", "error", err)
			message := "No se pudo conectar. Revisá que Jellyfin esté prendido."
			if errors.Is(err, jellyfin.ErrTokenRejected) {
				message = "Jellyfin rechazó el token. Volvé a vincular con Quick Connect."
			}
			s.render(w, r, templates.JellyfinTest(false, message))
			return
		}
		s.render(w, r, templates.JellyfinTest(true, "Conectado a "+serverName(info)+" "+info.Version))
		return
	}

	info, err := s.deps.Library.Reachable(ctx)
	switch {
	case errors.Is(err, jellyfin.ErrNoServerURL):
		s.render(w, r, templates.JellyfinTest(false, "Cargá primero la dirección de Jellyfin."))
	case err != nil:
		s.log.Warn("jellyfin reach", "error", err)
		s.render(w, r, templates.JellyfinTest(false,
			"No se llega a esa dirección. Revisá la IP y el puerto (el de Jellyfin suele ser 8096)."))
	default:
		s.render(w, r, templates.JellyfinTest(true,
			"Se llega a "+serverName(info)+" "+info.Version+", falta vincular con Quick Connect."))
	}
}

func serverName(info jellyfin.ServerInfo) string {
	if info.Name == "" {
		return "Jellyfin"
	}
	return info.Name
}

// jobFailureMessage turns a failed job into something the user can act on.
// A rejected token is called out by name because the fix is specific — link
// again — and because the generic message sends people to check their network,
// which has already cost a couple of rounds of debugging.
func jobFailureMessage(job jobs.Job, fallback string) string {
	if errors.Is(job.Cause, jellyfin.ErrTokenRejected) {
		return "Jellyfin rechazó el token. Volvé a vincular desde Ajustes."
	}
	return fallback
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
