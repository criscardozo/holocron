// Package httpserver wires the HTTP routes, middleware and templ rendering for
// Holocron.
package httpserver

import (
	"database/sql"
	"io"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/widgets"
	"github.com/cristian/holocron/web"
	"github.com/cristian/holocron/web/templates"
)

// Server holds the dependencies shared by the HTTP handlers.
type Server struct {
	log     *slog.Logger
	db      *sql.DB
	jobs    *jobs.Manager
	widgets *widgets.Registry
}

// New creates a Server.
func New(log *slog.Logger, database *sql.DB, jm *jobs.Manager, reg *widgets.Registry) *Server {
	return &Server{log: log, db: database, jobs: jm, widgets: reg}
}

// Handler builds the fully wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	staticFS, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		// Embedded FS layout is fixed at build time; a failure here is a bug.
		panic(err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /{$}", s.handleDashboard)
	mux.HandleFunc("GET /widgets/{id}", s.handleWidget)

	// Outermost first.
	return chain(mux, s.recoverer, s.logRequests, securityHeaders, gzipMW)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	grid := templates.Grid(s.widgets.Cards(r.Context()))
	s.render(w, r, templates.Dashboard(grid))
}

func (s *Server) handleWidget(w http.ResponseWriter, r *http.Request) {
	widget, ok := s.widgets.Get(r.PathValue("id"))
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
