package httpserver

import (
	"net/http"
	"time"

	"github.com/cristian/holocron/web/templates"
)

// themeCookie holds the scheme picked in Ajustes. A cookie rather than a
// setting in the database: it belongs to the browser, so the phone can be
// light while the computer stays dark.
const themeCookie = "holocron-tema"

// withTheme reads the picked scheme for the layout, which puts it on <html>
// before anything is painted: no flash of the other scheme, and no script.
func withTheme(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(themeCookie)
		if err != nil || !templates.ValidTheme(c.Value) {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(templates.WithTheme(r.Context(), c.Value)))
	})
}

// handleTheme stores the scheme and goes back to Ajustes. The form is not
// boosted: the scheme lives on <html>, which a boosted swap leaves alone.
func (s *Server) handleTheme(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	theme := r.PostFormValue("theme")
	if !templates.ValidTheme(theme) {
		http.Error(w, "unknown theme", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: themeCookie, Value: theme, Path: "/",
		MaxAge:   int((400 * 24 * time.Hour).Seconds()), // the most browsers keep
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
	http.Redirect(w, r, "/settings#apariencia", http.StatusSeeOther)
}
