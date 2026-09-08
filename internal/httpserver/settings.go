package httpserver

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/netaddr"
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

	// Each credential card says whether it is set up, instead of showing an
	// empty form that looks the same either way.
	view.Jellyfin = SettingsCredJellyfin(view)
	view.OpenSubs = SettingsCredOpenSubs(view)
	view.Qbit = SettingsCredQbit(view)
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
	// Normalised, not stored as typed: a bare "192.168.0.2:8080" is not a URL
	// and would fail every later call. Same trap as the Jellyfin address.
	address, err := netaddr.Normalise(r.PostFormValue("url"))
	if err != nil {
		s.log.Warn("save qbittorrent url", "error", err)
		s.redirect(w, r, "/settings?notice="+url.QueryEscape(
			"Esa dirección de qBittorrent no se entiende. Va algo como 127.0.0.1:8080."))
		return
	}
	if err := s.deps.Settings.Set(ctx, settings.KeyQbitURL, address); err != nil {
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

// The three credential cards. Configured means "there is enough stored here to
// try", never "it works": the live check is a separate, lazily-loaded answer,
// because those are different questions and conflating them is what made the
// old card misleading in the first place.

// SettingsCredJellyfin describes the Jellyfin card. Linked, not merely
// addressed: an address with no token is half-configured, and the Quick
// Connect step is exactly what the form is still there to offer.
func SettingsCredJellyfin(v templates.SettingsView) templates.SettingsCred {
	c := templates.SettingsCred{
		Configured: v.JellyfinLink.Linked,
		ClearHref:  "/settings/jellyfin/clear",
		Confirm:    "¿Borrar la dirección y el token de Jellyfin? Vas a tener que volver a vincularlo con Quick Connect.",
		StatusHref: "/settings/status/jellyfin",
	}
	if !c.Configured {
		return c
	}
	c.Facts = append(c.Facts, templates.SettingsFact{Label: "Servidor", Value: v.JellyfinURL})
	if v.JellyfinLink.User != "" {
		c.Facts = append(c.Facts, templates.SettingsFact{Label: "Cuenta", Value: v.JellyfinLink.User})
	}
	return c
}

// SettingsCredOpenSubs describes the OpenSubtitles card. No live check: there
// is no probe for it that does not spend a download from a small daily quota,
// and burning one to draw a badge would be a poor trade.
func SettingsCredOpenSubs(v templates.SettingsView) templates.SettingsCred {
	c := templates.SettingsCred{
		Configured: v.OpenSubsSet,
		ClearHref:  "/settings/opensubtitles/clear",
		Confirm:    "¿Borrar el usuario y la API key de OpenSubtitles?",
	}
	if !c.Configured {
		return c
	}
	if v.OpenSubsUser != "" {
		c.Facts = append(c.Facts, templates.SettingsFact{Label: "Usuario", Value: v.OpenSubsUser})
	}
	c.Facts = append(c.Facts, templates.SettingsFact{Label: "API key", Value: "guardada"})
	return c
}

// SettingsCredQbit describes the qBittorrent card.
func SettingsCredQbit(v templates.SettingsView) templates.SettingsCred {
	c := templates.SettingsCred{
		Configured: v.QbitURL != "" && v.QbitSet,
		ClearHref:  "/settings/qbittorrent/clear",
		Confirm:    "¿Borrar la URL, el usuario y la contraseña de qBittorrent?",
		StatusHref: "/settings/status/qbittorrent",
	}
	if !c.Configured {
		return c
	}
	c.Facts = append(c.Facts,
		templates.SettingsFact{Label: "WebUI", Value: v.QbitURL},
		templates.SettingsFact{Label: "Usuario", Value: v.QbitUser},
		templates.SettingsFact{Label: "Contraseña", Value: "guardada"},
	)
	return c
}

// handleClearJellyfin forgets the address and the token together.
//
// Both, not just one. Leaving the address behind after unlinking is what makes
// a card look configured while nothing works, and this button exists precisely
// to get back to a clean start.
func (s *Server) handleClearJellyfin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.deps.JellyfinLink.Unlink(ctx); err != nil {
		s.log.Warn("clear jellyfin", "error", err)
	}
	if err := s.deps.Settings.Set(ctx, settings.KeyJellyfinURL, ""); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, "/settings?notice="+url.QueryEscape("Credenciales de Jellyfin borradas."))
}

func (s *Server) handleClearOpenSubtitles(w http.ResponseWriter, r *http.Request) {
	if err := s.clearKeys(r, settings.KeyOpenSubtitlesUser, settings.KeyOpenSubtitlesKey); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, "/settings?notice="+url.QueryEscape("Credenciales de OpenSubtitles borradas."))
}

func (s *Server) handleClearQbit(w http.ResponseWriter, r *http.Request) {
	if err := s.clearKeys(r, settings.KeyQbitURL, settings.KeyQbitUser, settings.KeyQbitPass); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, "/settings?notice="+url.QueryEscape("Credenciales de qBittorrent borradas."))
}

// clearKeys empties every key or fails, so a half-cleared card cannot happen.
func (s *Server) clearKeys(r *http.Request, keys ...string) error {
	for _, k := range keys {
		if err := s.deps.Settings.Set(r.Context(), k, ""); err != nil {
			return err
		}
	}
	return nil
}

// handleJellyfinStatus and handleQbitStatus answer the "is it actually working"
// question the card asks after it has rendered.
func (s *Server) handleJellyfinStatus(w http.ResponseWriter, r *http.Request) {
	info, err := s.deps.Library.TestConnection(r.Context())
	if err != nil {
		s.log.Debug("jellyfin status", "error", err)
		s.render(w, r, templates.CredStatus(false, "no responde"))
		return
	}
	detail := "responde"
	if info.Name != "" {
		detail = info.Name
	}
	s.render(w, r, templates.CredStatus(true, detail))
}

func (s *Server) handleQbitStatus(w http.ResponseWriter, r *http.Request) {
	if _, err := s.deps.Torrents.Summary(r.Context()); err != nil {
		s.log.Debug("qbittorrent status", "error", err)
		s.render(w, r, templates.CredStatus(false, "no responde"))
		return
	}
	s.render(w, r, templates.CredStatus(true, "responde"))
}
