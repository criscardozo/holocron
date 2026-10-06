package httpserver

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/cristian/holocron/internal/stack"
	"github.com/cristian/holocron/web/templates"
)

func (s *Server) handleStack(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.StackPage(s.stackView(r.Context(), r.Host)))
}

// stackView puts together what each app is for, its two ways in, its version
// and whether its unit runs. Versions come from a cache and the unit states
// from the services hub, which reuses a fresh reading when there is one.
func (s *Server) stackView(ctx context.Context, host string) templates.StackView {
	domain := stack.Domain(host)
	var versions map[string]string
	if s.deps.Versions != nil {
		versions = s.deps.Versions.Get(ctx)
	}
	units := map[string]bool{}
	states := map[string]string{}
	if s.deps.Services != nil && s.deps.ServicesConfigured {
		for _, u := range s.deps.Services.Current(ctx).Units {
			units[u.Name] = u.OK()
			states[u.Name] = unitState(u)
		}
	}

	v := templates.StackView{Direct: s.deps.DirectHost != "", Watched: len(units) > 0}
	for _, a := range stack.Apps {
		app := templates.StackApp{
			Name: a.Name, Purpose: a.Purpose,
			URL: a.URL(domain), Host: strings.TrimSuffix(strings.TrimPrefix(a.URL(domain), "https://"), "/"),
			Logo:    "/static/apps/" + a.Logo,
			Version: versions[a.Key],
		}
		if s.deps.DirectHost != "" {
			app.DirectLabel = net.JoinHostPort(s.deps.DirectHost, strconv.Itoa(a.Port))
			app.Direct = "http://" + app.DirectLabel + "/"
		}
		if ok, watched := units[a.Unit]; watched {
			app.State, app.OK = states[a.Unit], ok
		}
		if a.Featured {
			v.Featured = append(v.Featured, app)
		} else {
			v.Others = append(v.Others, app)
		}
	}
	return v
}
