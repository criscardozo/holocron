package httpserver

import (
	"compress/gzip"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// middleware wraps an http.Handler with cross-cutting behaviour.
type middleware func(http.Handler) http.Handler

// chain applies middlewares so that the first argument is the outermost layer.
func chain(h http.Handler, mws ...middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// recoverer turns a handler panic into a 500 instead of crashing the process.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic recovered", "path", r.URL.Path, "error", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// logRequests emits one structured log line per request.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// maxRequestBody caps how much of a request body a handler will read. Every
// form this app posts is a few hundred bytes at most; the limit keeps a large
// upload from being buffered into memory on a small device.
const maxRequestBody = 1 << 20 // 1 MiB

// limitBody caps the size of request bodies before any handler parses a form.
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		}
		next.ServeHTTP(w, r)
	})
}

// sameOrigin rejects state-changing requests that a browser tells us came from
// somewhere else.
//
// The web UI has no session, so there is nothing for a request to prove: every
// POST that reaches a handler is acted on. That makes any page open in a
// browser on the LAN able to drive Holocron — measured before this existed, a
// POST to /settings/folders carrying Origin: https://evil.example returned 303
// and the folder was written. Cloudflare Access does not help: the request
// comes from a browser that already holds a valid session, or straight to
// :8090 on the LAN.
//
// The check is on the two headers a browser attaches and a page cannot forge.
// Sec-Fetch-Site is preferred because it is unambiguous; Origin is the fallback
// for anything that omits it. A request with neither is not from a browser
// — curl, the installer, a script — and is left alone, which is also what keeps
// the API usable: it authenticates with a bearer token instead.
func (s *Server) sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !changesState(r.Method) || strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
			// "none" is a direct navigation, e.g. a bookmarked POST target.
			if site != "same-origin" && site != "none" {
				s.rejectCrossSite(w, r, "sec-fetch-site", site)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !sameHost(origin, r.Host) {
			s.rejectCrossSite(w, r, "origin", origin)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func changesState(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// sameHost compares an Origin header against the host the request was sent to.
// Only the host is compared, never the scheme: behind the Cloudflare tunnel the
// browser sends https while the origin server speaks http, and requiring them
// to match would reject every request that arrives through it.
func sameHost(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == host
}

func (s *Server) rejectCrossSite(w http.ResponseWriter, r *http.Request, header, value string) {
	// Logged as a warning with the value as an attribute: this is either a
	// misconfigured proxy or someone trying, and both are worth seeing.
	s.log.Warn("rejected cross-site request",
		"method", r.Method, "path", r.URL.Path, header, value)
	http.Error(w, "cross-site requests are not accepted", http.StatusForbidden)
}

// securityHeaders sets a strict, same-origin security policy. Everything the
// page needs (HTMX, CSS) is served from this origin, so 'self' suffices.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// gzipMW compresses HTML responses. Static assets are skipped: the file server
// sets Content-Length up front, which would disagree with a compressed body.
func gzipMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") ||
			!strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		gz := gzip.NewWriter(w)
		defer func() { _ = gz.Close() }()
		next.ServeHTTP(gzipWriter{ResponseWriter: w, gz: gz}, r)
	})
}

type gzipWriter struct {
	http.ResponseWriter
	gz *gzip.Writer
}

func (g gzipWriter) Write(b []byte) (int, error) { return g.gz.Write(b) }

// statusRecorder captures the response status code for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.wrote = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.wrote = true
	}
	return r.ResponseWriter.Write(b)
}
