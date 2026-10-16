package httpserver

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/cristian/holocron/internal/version"
	"github.com/cristian/holocron/web"
	"github.com/cristian/holocron/web/templates"
)

// The pieces that make the web installable as an app on the phone. They live
// at the root rather than under /static/: a service worker only controls the
// pages under its own path, and this one has to cover them all.

// offlineAssets are what the offline page needs, cached by the service worker
// when it installs. The same versioned URLs the pages link, so a release
// replaces them.
func offlineAssets() []string {
	return []string{templates.Asset("styles.css"), templates.Asset("app.js"), "/static/ginebra.svg"}
}

func (s *Server) handleManifest(w http.ResponseWriter, _ *http.Request) {
	body, err := fs.ReadFile(web.StaticFS, "static/manifest.webmanifest")
	if err != nil {
		s.log.Error("read manifest", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if _, err := w.Write(body); err != nil {
		s.log.Debug("write manifest", "error", err)
	}
}

// handleServiceWorker serves sw.js with the release's version and asset URLs
// in front. A browser compares the worker byte for byte, so a new release is
// a new worker, and the URLs it caches are the ones this release links.
func (s *Server) handleServiceWorker(w http.ResponseWriter, _ *http.Request) {
	body, err := fs.ReadFile(web.StaticFS, "static/sw.js")
	if err != nil {
		s.log.Error("read service worker", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ver, err := json.Marshal(version.Current())
	if err != nil {
		s.log.Error("encode version", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	assets, err := json.Marshal(offlineAssets())
	if err != nil {
		s.log.Error("encode offline assets", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	// Always asked again: a cached worker would hold the app on an old
	// release.
	w.Header().Set("Cache-Control", "no-cache")
	if _, err := fmt.Fprintf(w, "const VERSION = %s;\nconst OFFLINE_ASSETS = %s;\n\n%s", ver, assets, body); err != nil {
		s.log.Debug("write service worker", "error", err)
	}
}

func (s *Server) handleOffline(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.OfflinePage())
}
