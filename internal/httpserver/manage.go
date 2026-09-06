package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/cristian/holocron/internal/apitoken"
	"github.com/cristian/holocron/internal/power"
	"github.com/cristian/holocron/internal/system"
	"github.com/cristian/holocron/internal/widgets"
	"github.com/cristian/holocron/web/templates"
)

// The management screen acts on the machine itself. Holocron runs unprivileged
// and cannot restart or stop anything; it signals a root path unit, one per
// action. See internal/power for why the signal carries no content.

func (s *Server) handleManagePage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.ManagePage(s.manageView(r.Context(), templates.ManagePageView{})))
}

// handleManageAction runs one action. The whole exchange is deliberately
// fire-and-acknowledge: the request that asks the machine to stop cannot also
// report how it went, because the process answering it is about to be stopped.
func (s *Server) handleManageAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	fail := func(message string) {
		view := s.manageView(ctx, templates.ManagePageView{Notice: message, NoticeErr: true})
		s.render(w, r, templates.ManageSection(view))
	}

	action, ok := power.Valid(strings.TrimSpace(r.PostFormValue("action")))
	if !ok {
		s.log.Warn("rejected machine action", "reason", "unknown action")
		fail("Esa acción no existe.")
		return
	}

	// The token is the friction on the one action that cannot be undone
	// remotely. It is checked here rather than trusted from a session, because
	// there are no sessions: on the LAN the web has no authentication at all.
	if action.NeedsToken() {
		token := strings.TrimSpace(r.PostFormValue("token"))
		if token == "" {
			fail("Pegá el token de la API para confirmar.")
			return
		}
		if err := s.deps.APIToken.Verify(ctx, token); err != nil {
			if errors.Is(err, apitoken.ErrNoToken) {
				fail("No hay token generado. Generá uno en Ajustes → App iOS.")
				return
			}
			s.log.Warn("rejected machine action", "action", string(action), "reason", "invalid token")
			fail("El token no coincide.")
			return
		}
	}

	if err := s.deps.Power.Request(action); err != nil {
		s.log.Warn("machine action", "action", string(action), "error", err)
		if errors.Is(err, power.ErrNoHelper) {
			fail("El ayudante con privilegios no está instalado. Volvé a correr el instalador.")
			return
		}
		fail("No se pudo pedir la acción.")
		return
	}

	s.log.Info("machine action requested", "action", string(action))
	view := s.manageView(ctx, templates.ManagePageView{
		Notice: "Pedido: " + action.Label() + ". " + afterword(action),
	})
	s.render(w, r, templates.ManageSection(view))
}

// afterword tells the user what to expect from a request whose result they may
// never see, because the thing acting on it is what serves this page.
func afterword(a power.Action) string {
	switch a {
	case power.ActionPowerOff:
		return "Cuando esta página deje de responder, ya está apagada."
	case power.ActionReboot:
		return "Va a dejar de responder un minuto o dos y vuelve sola."
	case power.ActionRestartHolocron:
		return "Recargá en unos segundos."
	case power.ActionRestartCloudflare:
		return "Si entraste por el dominio público, puede cortarse la conexión."
	default:
		return "Vuelve solo en unos segundos."
	}
}

// manageView assembles the screen. base carries anything the caller already
// decided, such as a notice.
func (s *Server) manageView(ctx context.Context, base templates.ManagePageView) templates.ManagePageView {
	v := base
	v.Available = s.deps.Power.Installed()

	v.Host = widgets.SystemViewOf(system.Read())

	jellyfinConfigured := s.deps.Library.Configured(ctx)
	v.Jellyfin = templates.ManageServiceRow{Name: "Jellyfin", Configured: jellyfinConfigured}
	if jellyfinConfigured {
		_, err := s.deps.Library.TestConnection(ctx)
		v.Jellyfin.Reachable = err == nil
	}

	torrentsConfigured := s.deps.Torrents.Configured(ctx)
	v.Torrents = templates.ManageServiceRow{Name: "qBittorrent", Configured: torrentsConfigured}
	if torrentsConfigured {
		_, err := s.deps.Torrents.Summary(ctx)
		v.Torrents.Reachable = err == nil
	}

	// What would be interrupted. Best effort by design: a probe that cannot
	// answer withholds a warning rather than blocking the page.
	pre := power.Check(ctx, s.deps.Library, s.deps.Torrents)
	v.Warnings, v.Checked = pre.Warnings, pre.Checked

	if pending, ok := s.deps.Power.Pending(); ok {
		v.Pending = pending.Label() + " — pedido, esperando a que el sistema lo tome…"
	}

	for _, a := range power.Actions {
		if !s.deps.Power.Available(a) {
			continue
		}
		v.Actions = append(v.Actions, templates.ManageActionRow{
			Key:         string(a),
			Label:       a.Label(),
			Detail:      a.Detail(),
			Confirm:     a.Confirm(),
			Icon:        a.Icon(),
			Destructive: a.NeedsToken(),
		})
	}
	return v
}
