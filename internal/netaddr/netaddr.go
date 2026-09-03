// Package netaddr normalises the server addresses a person types into the
// settings form.
//
// "192.168.0.2:8096" is what anyone writes down for a machine on their LAN,
// and it is not a URL: net/url reads the colon as a scheme separator and
// rejects it with "first path segment in URL cannot contain colon". Stored as
// typed, every later request fails before a packet leaves the machine, and the
// UI can only report a generic "could not connect" — which sends the user
// looking at their network instead of at the missing http://.
package netaddr

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ErrBadAddress means the text is not something that can be requested.
var ErrBadAddress = errors.New("that does not look like a server address")

// Repair makes the address requestable without judging it: it assumes http://
// when no scheme was given and drops a trailing slash. Used by the clients so
// an install that already stored a bare host:port starts working on upgrade,
// without waiting for someone to re-save the form.
func Repair(raw string) string {
	s := strings.TrimSpace(raw)
	if !strings.Contains(s, "://") {
		s = "http://" + strings.TrimLeft(s, "/")
		if s == "http://" {
			return ""
		}
	}
	return strings.TrimRight(s, "/")
}

// Normalise repairs and validates, for the write path: a typo is rejected while
// the person who made it is still looking at the form. An empty address is
// allowed and means "not set". A path is kept — these services are often served
// under a subpath behind a reverse proxy.
func Normalise(raw string) (string, error) {
	base := Repair(raw)
	if base == "" {
		return "", nil
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrBadAddress, raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%w: %q", ErrBadAddress, raw)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return "", fmt.Errorf("%w: %q", ErrBadAddress, raw)
	}
	// Credentials, query and fragment cannot mean anything on a base address,
	// and keeping them would repeat them on every request.
	return (&url.URL{
		Scheme: strings.ToLower(u.Scheme),
		Host:   u.Host,
		Path:   strings.TrimRight(u.Path, "/"),
	}).String(), nil
}
