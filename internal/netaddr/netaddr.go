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
	"net"
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

// IsPrivateHost reports whether host names a machine only reachable from
// inside a home network. The port is optional and ignored.
//
// This exists to tell "you are standing next to the machine" apart from "you
// are somewhere else", which is the difference that matters before an action
// that cannot be undone remotely. It is emphatically not a security boundary:
// the Host header is client-supplied, so a private-looking value proves
// nothing. It is used only to decide how much friction to put in front of a
// button, never to grant access.
func IsPrivateHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return false
	}
	// Host headers and typed addresses both arrive with a port more often than
	// not, and an IPv6 literal arrives in brackets.
	if stripped, _, err := net.SplitHostPort(h); err == nil {
		h = stripped
	}
	h = strings.Trim(h, "[]")

	if ip := net.ParseIP(h); ip != nil {
		// IsPrivate covers 10/8, 172.16/12, 192.168/16 and fc00::/7; the other
		// two catch localhost by address and the 169.254 self-assigned range a
		// machine uses when DHCP did not answer.
		//
		// Note what is missing: 100.64/10, the range Tailscale hands out. It is
		// a private network but not a *nearby* one — reaching the machine over
		// a VPN from another country looks identical to reaching it from the
		// couch, so it is treated as remote. That is the safe direction, and
		// here it is also the honest one.
		return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
	}
	// Not an address, so a name. Only the names that cannot resolve outside a
	// local network count; anything else is treated as public, which is the
	// safe direction to be wrong in.
	return h == "localhost" || strings.HasSuffix(h, ".local") ||
		strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".home.arpa")
}
