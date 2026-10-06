// Package stack describes the applications that run on the server beside
// Holocron, and reads their versions.
//
// It replaces the server's static portal: the start page links to each app,
// and the Stack page says what each one is for, how to reach it by name and
// directly by address, which version runs and whether it is up.
package stack

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// App is one application of the stack.
type App struct {
	Key  string // stable slug; also the logo's file name stem
	Name string
	// Short is the one-liner of the start page; Purpose the fuller sentence
	// of the Stack page.
	Short, Purpose string
	// Sub is the subdomain it answers on, behind the same proxy as Holocron.
	Sub  string
	Port int    // where it listens on the server, for direct access
	Logo string // file under /static/apps/
	// Unit is its systemd service, to read whether it runs.
	Unit string
	// Featured apps are the ones the house uses every day; the rest are the
	// machinery behind them.
	Featured bool
}

// Apps is the stack, in display order.
var Apps = []App{
	{Key: "jellyfin", Name: "Jellyfin", Short: "Películas y series",
		Purpose: "El servidor de películas y series: lo que se mira en la tele, el teléfono o el navegador.",
		Sub:     "jellyfin", Port: 8096, Logo: "jellyfin.svg", Unit: "jellyfin", Featured: true},
	{Key: "seerr", Name: "Seerr", Short: "Pedir títulos nuevos",
		Purpose: "Donde se pide lo que todavía no está. Lo pedido pasa solo a Radarr o a Sonarr.",
		Sub:     "seerr", Port: 5055, Logo: "seerr.png", Unit: "jellyseerr", Featured: true},
	{Key: "radarr", Name: "Radarr", Short: "Consigue las películas",
		Purpose: "Busca las películas pedidas, las baja y las ordena en la biblioteca.",
		Sub:     "radarr", Port: 7878, Logo: "radarr.png", Unit: "radarr"},
	{Key: "sonarr", Name: "Sonarr", Short: "Consigue las series",
		Purpose: "Lo mismo para las series: sigue cada temporada y baja los episodios nuevos.",
		Sub:     "sonarr", Port: 8989, Logo: "sonarr.png", Unit: "sonarr"},
	{Key: "prowlarr", Name: "Prowlarr", Short: "Dónde buscar todo",
		Purpose: "Administra los indexadores, los lugares donde Radarr y Sonarr buscan.",
		Sub:     "prowlarr", Port: 9696, Logo: "prowlarr.png", Unit: "prowlarr"},
	{Key: "bazarr", Name: "Bazarr", Short: "Consigue los subtítulos",
		Purpose: "Busca los subtítulos de lo que ya está en la biblioteca.",
		Sub:     "bazarr", Port: 6767, Logo: "bazarr.png", Unit: "bazarr"},
	{Key: "trailarr", Name: "Trailarr", Short: "Consigue los trailers",
		Purpose: "Baja los trailers de las películas y series de la biblioteca.",
		Sub:     "trailarr", Port: 7889, Logo: "trailarr.png", Unit: "trailarr"},
	{Key: "qbittorrent", Name: "qBittorrent", Short: "Herramienta para bajar",
		Purpose: "El cliente que hace las descargas que piden Radarr y Sonarr.",
		Sub:     "qb", Port: 8080, Logo: "qbittorrent.svg", Unit: "qbittorrent"},
}

// Domain is the domain the apps answer under, from the host a request came
// to: holocron.merli.store and merli.store both mean merli.store. Derived per
// request rather than configured, so Holocron serves the same links under
// either name and no domain is written into the code.
func Domain(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimPrefix(strings.ToLower(host), "holocron.")
}

// URL is an app's address by name on that domain.
func (a App) URL(domain string) string {
	return "https://" + a.Sub + "." + domain + "/"
}

// DirectHost is the server's address on the home network, for reaching an app
// by IP and port when the proxy does not answer: the first private IPv4 of an
// interface that is up. Tailscale's range (100.64.0.0/10) is not private and is
// skipped by the same test. Empty when there is none.
func DirectHost() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			pfx, err := netip.ParsePrefix(a.String())
			if err != nil {
				continue
			}
			if ip := pfx.Addr(); ip.Is4() && ip.IsPrivate() {
				return ip.String()
			}
		}
	}
	return ""
}

// VersionFunc reads one app's running version.
type VersionFunc func(context.Context) (string, error)

// versionsTTL is how long versions are kept. They change when an app is
// upgraded, a few times a month; the Stack page should not ask seven services
// on every visit.
const versionsTTL = 30 * time.Minute

// versionTimeout bounds each app's answer: one that hangs must not hold up
// the page.
const versionTimeout = 3 * time.Second

// Versions reads and caches the apps' versions.
type Versions struct {
	fetch map[string]VersionFunc

	mu   sync.Mutex
	have map[string]string
	at   time.Time
}

// NewVersions builds a cache over one reader per app key. Apps without one
// (Trailarr wants a login Holocron does not have) show no version.
func NewVersions(fetch map[string]VersionFunc) *Versions {
	return &Versions{fetch: fetch, have: map[string]string{}}
}

// Get returns what is known, reading every app at once when the cache is
// stale. An app that does not answer keeps its last known version.
func (v *Versions) Get(ctx context.Context) map[string]string {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.at.IsZero() && time.Since(v.at) < versionsTTL {
		return v.copy()
	}
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		got = map[string]string{}
	)
	for key, f := range v.fetch {
		wg.Go(func() {
			cctx, cancel := context.WithTimeout(ctx, versionTimeout)
			defer cancel()
			if ver, err := f(cctx); err == nil && ver != "" {
				mu.Lock()
				got[key] = strings.TrimPrefix(ver, "v")
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	for k, ver := range got {
		v.have[k] = ver
	}
	v.at = time.Now()
	return v.copy()
}

func (v *Versions) copy() map[string]string {
	out := make(map[string]string, len(v.have))
	for k, ver := range v.have {
		out[k] = ver
	}
	return out
}
