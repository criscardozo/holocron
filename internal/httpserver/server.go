// Package httpserver wires the HTTP routes, middleware and templ rendering for
// Holocron.
package httpserver

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/cristian/holocron/internal/diskusage"
	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/widgets"
	"github.com/cristian/holocron/web"
	"github.com/cristian/holocron/web/templates"
)

// Deps are the dependencies shared by the HTTP handlers. Later phases add their
// own services here.
type Deps struct {
	Log     *slog.Logger
	Widgets *widgets.Registry
	Folders *folders.Store
	Disk    *diskusage.Service
}

// Server serves the Holocron web UI.
type Server struct {
	deps Deps
	log  *slog.Logger
}

// New creates a Server.
func New(d Deps) *Server {
	return &Server{deps: d, log: d.Log}
}

// Handler builds the fully wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	staticFS, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		panic(err) // embedded layout is fixed at build time
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /{$}", s.handleDashboard)
	mux.HandleFunc("GET /widgets/{id}", s.handleWidget)

	// Phase 1: disk usage.
	mux.HandleFunc("GET /disk", s.handleDiskPage)
	mux.HandleFunc("POST /disk/scan", s.handleDiskScan)
	mux.HandleFunc("GET /disk/status", s.handleDiskStatus)
	mux.HandleFunc("GET /disk/browse", s.handleDiskBrowse)

	// Phase 1: settings (watched folders).
	mux.HandleFunc("GET /settings", s.handleSettings)
	mux.HandleFunc("POST /settings/folders", s.handleAddFolder)
	mux.HandleFunc("POST /settings/folders/delete", s.handleDeleteFolder)

	return chain(mux, s.recoverer, s.logRequests, securityHeaders, gzipMW)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	grid := templates.Grid(s.deps.Widgets.Cards(r.Context()))
	s.render(w, r, templates.Dashboard(grid))
}

func (s *Server) handleWidget(w http.ResponseWriter, r *http.Request) {
	widget, ok := s.deps.Widgets.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.render(w, r, widget.Card(r.Context()))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "ok")
}

// render writes an HTML component. On a render error we can only log it: the
// status and part of the body may already be on the wire.
func (s *Server) render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(r.Context(), w); err != nil {
		s.log.Error("render component", "path", r.URL.Path, "error", err)
	}
}

// redirect sends the client to url. For HTMX requests it uses HX-Redirect so
// the browser performs a full navigation; otherwise a 303.
func (s *Server) redirect(w http.ResponseWriter, r *http.Request, url string) {
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", url)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}
