package httpserver

import (
	"net/http"
	"strconv"
)

// serverError logs the detail server-side and returns a generic 500 so no
// internal detail leaks to the client.
func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("handler error", "path", r.URL.Path, "error", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

// formInt64 parses a required int64 form field, writing a 400 on failure.
func (s *Server) formInt64(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return 0, false
	}
	v, err := strconv.ParseInt(r.PostFormValue(name), 10, 64)
	if err != nil {
		http.Error(w, "bad "+name, http.StatusBadRequest)
		return 0, false
	}
	return v, true
}

// queryInt64 parses an int64 query parameter.
func queryInt64(r *http.Request, name string) (int64, bool) {
	v, err := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
