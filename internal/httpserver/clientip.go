package httpserver

import (
	"net"
	"net/http"
	"strings"

	"github.com/cristian/holocron/internal/netaddr"
)

// clientIP is the address the request really came from.
//
// On Ginebra, Holocron listens on loopback behind Caddy, so RemoteAddr is
// always 127.0.0.1 and the real client is in X-Forwarded-For. That header is
// believed only when the connection itself comes from loopback — anyone
// reaching the port directly could write whatever they like in it — and even
// then only its last entry, the one Caddy appended. Earlier entries came from
// the client and prove nothing.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return host
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return host
	}
	parts := strings.Split(xff, ",")
	if last := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(last) != nil {
		return last
	}
	return host
}

// fromHome reports whether the request came from the house's own network.
//
// It used to read the Host header, which made sense while a Cloudflare tunnel
// sat in front: the tunnel's hostname meant "from the internet". Behind Caddy
// every request carries the same Host, so the only honest signal left is the
// client's address. That is also a better one: the Host header was entirely
// client-supplied, while X-Forwarded-For is trusted only from the local proxy.
//
// Tailscale's 100.64/10 still counts as away, on purpose — reaching the
// machine over the VPN from another country looks exactly like reaching it
// from the couch. See netaddr.IsPrivateHost.
func fromHome(r *http.Request) bool {
	return netaddr.IsPrivateHost(clientIP(r))
}
