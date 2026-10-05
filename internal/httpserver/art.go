package httpserver

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cristian/holocron/internal/artwork"
	"github.com/cristian/holocron/web/templates"
)

// muralPosters is how many different posters the mural draws from, and
// muralSlots how many tiles it lays out, repeating them. The same numbers as
// Ginebra's portal: enough to fill a wide screen without asking for more than
// a couple dozen images.
const (
	muralPosters = 28
	muralSlots   = 72
	muralTTL     = 15 * time.Minute
)

// mural keeps the posters behind the pages for a while, so moving between
// pages shows the same wall instead of a new one each time.
type mural struct {
	mu   sync.Mutex
	urls []string
	at   time.Time
}

func (s *Server) muralURLs(ctx context.Context) []string {
	s.mural.mu.Lock()
	defer s.mural.mu.Unlock()
	if s.mural.urls != nil && time.Since(s.mural.at) < muralTTL {
		return s.mural.urls
	}
	ids, err := s.deps.Library.PosterIDs(ctx, muralPosters)
	if err != nil {
		s.log.Warn("mural posters unavailable", "err", err)
		return s.mural.urls
	}
	var urls []string
	for _, id := range ids {
		if u := artwork.URL(artwork.KindJellyfin, id); u != "" {
			urls = append(urls, u)
		}
	}
	if len(urls) > 0 {
		filled := make([]string, muralSlots)
		for i := range filled {
			filled[i] = urls[i%len(urls)]
		}
		// Shuffled once the wall is full, so repeats do not line up in
		// columns the way a plain modulo would put them.
		rand.Shuffle(len(filled), func(i, j int) { filled[i], filled[j] = filled[j], filled[i] }) //#nosec G404 -- arranging posters, nothing to guess
		urls = filled
	}
	s.mural.urls, s.mural.at = urls, time.Now()
	return urls
}

// withMural hands the mural to the layout through the request context, so no
// page handler has to know it exists. Only pages get it: fragments, streams
// and assets have no layout to put it in.
func (s *Server) withMural(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || s.deps.Library == nil || !isPage(r) {
			next.ServeHTTP(w, r)
			return
		}
		ctx := templates.WithMural(r.Context(), s.muralURLs(r.Context()))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// isPage is whether a request renders a full page rather than a fragment.
func isPage(r *http.Request) bool {
	for _, p := range []string{"/static/", "/art/", "/events/", "/api/", "/widgets/", "/healthz"} {
		if strings.HasPrefix(r.URL.Path, p) {
			return false
		}
	}
	// An htmx request swaps a part of a page; a boosted one is a navigation
	// and takes the whole body, mural included.
	return r.Header.Get("HX-Request") == "" || r.Header.Get("HX-Boosted") != ""
}

// blankPoster is what a poster that is not there looks like: the placeholder's
// gradient, in the poster's proportions.
const blankPoster = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 300">` +
	`<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1">` +
	`<stop offset="0" stop-color="#1d2432"/><stop offset="1" stop-color="#1a1430"/>` +
	`</linearGradient></defs><rect width="200" height="300" fill="url(#g)"/></svg>`

// handleArt serves a poster from the artwork cache.
func (s *Server) handleArt(w http.ResponseWriter, r *http.Request) {
	if s.deps.Art == nil || !artwork.Valid(r.PathValue("kind"), r.PathValue("id")) {
		http.NotFound(w, r)
		return
	}
	img, err := s.deps.Art.Get(r.Context(), r.PathValue("kind"), r.PathValue("id"))
	if err != nil {
		if !errors.Is(err, artwork.ErrNotFound) {
			s.log.Warn("poster unavailable", "kind", r.PathValue("kind"), "err", err)
		}
		// A blank poster rather than a 404: a failed <img> shows the browser's
		// broken-image icon, and swapping it out would take an onerror
		// handler the CSP forbids. Cached briefly, so a poster that turns up
		// later is not hidden behind it for a week.
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		if _, err := io.WriteString(w, blankPoster); err != nil {
			s.log.Debug("poster write", "err", err)
		}
		return
	}
	w.Header().Set("Content-Type", img.Type)
	w.Header().Set("Content-Length", strconv.Itoa(len(img.Data)))
	// A week in the browser: the URL names the item, not the image, so it
	// cannot be immutable — Jellyfin's poster can change.
	w.Header().Set("Cache-Control", "public, max-age=604800")
	if _, err := w.Write(img.Data); err != nil {
		s.log.Debug("poster write", "err", err)
	}
}
