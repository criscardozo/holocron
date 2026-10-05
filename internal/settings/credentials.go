package settings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrManaged means the setting comes from the server and cannot be changed from
// the UI. Reported rather than ignored, so a form that tries says why it did
// nothing.
var ErrManaged = errors.New("managed by the server")

// On Ginebra the keys of the neighbouring services live in /etc/ginebra/keys/,
// root-owned and 0600, and systemd hands them to Holocron with LoadCredential=.
// They arrive as files in $CREDENTIALS_DIRECTORY, a private directory only this
// service can read. That keeps the rule the house runs on: keys stay on the
// server, never pass through a form, and the browser never sees them.
//
// Outside Ginebra none of this exists and the settings form works as before,
// so the binary stays usable anywhere.

// Credential names, as Ginebra writes them: one file per app, the bare value.
const (
	CredJellyfin    = "jellyfin"
	CredQbittorrent = "qbittorrent" // two lines: user, then password
	CredRadarr      = "radarr"
	CredSonarr      = "sonarr"
	CredProwlarr    = "prowlarr"
	CredSeerr       = "seerr"
	CredBazarr      = "bazarr"
)

// Credentials are the values read from $CREDENTIALS_DIRECTORY.
type Credentials map[string]string

// LoadCredentials reads every file in dir. A missing dir is not an error: it is
// what running outside Ginebra looks like.
func LoadCredentials(dir string) (Credentials, error) {
	if dir == "" {
		return Credentials{}, nil
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make(Credentials, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name())) //#nosec G304 -- names come from systemd's own credentials directory
		if err != nil {
			return nil, err
		}
		out[e.Name()] = strings.TrimRight(string(b), "\r\n")
	}
	return out, nil
}

// Manage installs the credentials as read-only settings.
//
// Only what Holocron already stored as settings is mapped here: Jellyfin and
// qBittorrent. The *arr keys are read directly by their clients through Get.
// Service addresses default to loopback, where Ginebra runs them, unless a
// value was saved by hand.
func (s *Store) Manage(c Credentials) {
	m := map[string]string{}
	if v := c[CredJellyfin]; v != "" {
		m[KeyJellyfinToken] = v
		// An API key carries the server's own authority: there is no user
		// behind it to name, and it can do what an administrator can.
		m[KeyJellyfinUser] = "API key del servidor"
		m[KeyJellyfinAdmin] = "true"
	}
	if v := c[CredQbittorrent]; v != "" {
		user, pass, _ := strings.Cut(v, "\n")
		m[KeyQbitUser] = strings.TrimSpace(user)
		m[KeyQbitPass] = strings.TrimRight(pass, "\r\n")
	}
	for name, v := range c {
		m["cred."+name] = v
	}
	s.managed = m
	s.defaults = map[string]string{}
	if _, ok := m[KeyJellyfinToken]; ok {
		s.defaults[KeyJellyfinURL] = "http://127.0.0.1:8096"
	}
	if _, ok := m[KeyQbitUser]; ok {
		s.defaults[KeyQbitURL] = "http://127.0.0.1:8080"
	}
}

// Managed reports whether a setting comes from the server.
func (s *Store) Managed(key string) bool {
	_, ok := s.managed[key]
	return ok
}

// Credential returns a raw credential by name, for the clients of services
// Holocron never stored settings for.
func (s *Store) Credential(name string) (string, bool) {
	v, ok := s.managed["cred."+name]
	return v, ok && v != ""
}
