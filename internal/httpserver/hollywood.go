package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/cristian/holocron/internal/stack"
	"github.com/cristian/holocron/web/templates"
)

// handleHollywood is the TV launcher's settings, as a page of this site: what
// it configures and where, and the way into its own settings page. Holocron
// holds none of its secrets and cannot change anything there: every change
// on the launcher asks for the code the TV shows.
func (s *Server) handleHollywood(w http.ResponseWriter, r *http.Request) {
	v := templates.HollywoodView{
		URL: "https://hollywood." + stack.Domain(r.Host) + "/",
	}
	if s.deps.HollywoodURL != "" {
		v.Checked = true
		v.Up = launcherAnswers(r.Context(), s.deps.HollywoodURL)
	}
	s.render(w, r, templates.HollywoodPage(v))
}

// launcherAnswers asks the launcher's settings page for its front page, which
// needs no code. The launcher lives in the TV's own process, so «no answer»
// usually means the TV is off or the launcher is not the home screen.
func launcherAnswers(ctx context.Context, base string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req) //#nosec G107 -- the address is server configuration
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode < 500
}
