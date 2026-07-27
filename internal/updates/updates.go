// Package updates checks GitHub for a newer Holocron release and asks the
// privileged helper to install it.
//
// The service itself deliberately cannot update in place: it runs unprivileged
// under systemd with ProtectSystem=strict and NoNewPrivileges, so /usr/local/bin
// is read-only to it. Instead it drops a trigger file in its state directory;
// a root-owned path unit notices and runs the installer. See
// packaging/holocron-update.{path,service}.
package updates

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cristian/holocron/internal/version"
)

const (
	releasesURL = "https://api.github.com/repos/criscardozo/holocron/releases/latest"

	// cacheTTL keeps repeated page loads from hammering GitHub, which allows
	// 60 unauthenticated requests an hour per IP.
	cacheTTL = 30 * time.Minute

	// triggerName is the file the privileged helper watches for.
	triggerName = ".update-requested"

	// updaterUnit exists only when the installer set up the helper.
	updaterUnit = "/etc/systemd/system/holocron-update.path"

	maxBody = 1 << 20
)

// Release is the newest published release.
type Release struct {
	Tag       string
	Notes     string
	URL       string
	Published time.Time
}

// Status is what the settings page renders.
type Status struct {
	Current string
	// Latest is empty when the check has not run or failed.
	Latest    string
	Notes     string
	URL       string
	Available bool
	// Checkable is false for a dev build: there is no tag to compare against.
	Checkable bool
	// Installable is false when the privileged helper is not installed, in
	// which case the UI shows the manual command instead of a button.
	Installable bool
	Error       string
	CheckedAt   time.Time
}

// Service checks for releases and requests installation.
type Service struct {
	stateDir string
	hc       *http.Client

	// url and unitPath are swapped in tests.
	url      string
	unitPath string

	mu     sync.Mutex
	cached *Release
	at     time.Time
}

// NewService creates a Service that drops its trigger file in stateDir (the
// directory holding the database, which the service can write).
func NewService(stateDir string) *Service {
	return &Service{
		stateDir: stateDir,
		hc:       &http.Client{Timeout: 15 * time.Second},
		url:      releasesURL,
		unitPath: updaterUnit,
	}
}

// Installable reports whether the privileged helper is present.
func (s *Service) Installable() bool {
	_, err := os.Stat(s.unitPath)
	return err == nil
}

// Cached returns what is already known without contacting GitHub, so opening
// the settings page is instant and works with no internet. The user gets a
// "check now" button to go and look.
func (s *Service) Cached() Status {
	st := Status{
		Current:     version.Current(),
		Checkable:   version.IsRelease(),
		Installable: s.Installable(),
	}
	s.mu.Lock()
	rel, at := s.cached, s.at
	s.mu.Unlock()
	if rel == nil {
		return st
	}
	st.Latest, st.Notes, st.URL, st.CheckedAt = rel.Tag, rel.Notes, rel.URL, at
	st.Available = st.Checkable && newer(rel.Tag, st.Current)
	return st
}

// Status returns the current state, consulting GitHub at most once per
// cacheTTL. force skips the cache (the "check now" button).
func (s *Service) Status(ctx context.Context, force bool) Status {
	st := Status{
		Current:     version.Current(),
		Checkable:   version.IsRelease(),
		Installable: s.Installable(),
	}
	if !st.Checkable {
		return st
	}

	rel, at, err := s.latest(ctx, force)
	if err != nil {
		st.Error = "No se pudo consultar GitHub."
		return st
	}
	st.Latest, st.Notes, st.URL, st.CheckedAt = rel.Tag, rel.Notes, rel.URL, at
	st.Available = newer(rel.Tag, st.Current)
	return st
}

func (s *Service) latest(ctx context.Context, force bool) (Release, time.Time, error) {
	s.mu.Lock()
	if !force && s.cached != nil && time.Since(s.at) < cacheTTL {
		rel, at := *s.cached, s.at
		s.mu.Unlock()
		return rel, at, nil
	}
	s.mu.Unlock()

	rel, err := s.fetch(ctx)
	if err != nil {
		return Release{}, time.Time{}, err
	}

	now := time.Now()
	s.mu.Lock()
	s.cached, s.at = &rel, now
	s.mu.Unlock()
	return rel, now, nil
}

func (s *Service) fetch(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "holocron")

	resp, err := s.hc.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("query releases: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Release{}, fmt.Errorf("github returned %s", resp.Status)
	}

	var payload struct {
		TagName     string    `json:"tag_name"`
		Body        string    `json:"body"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
		Draft       bool      `json:"draft"`
		Prerelease  bool      `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&payload); err != nil {
		return Release{}, fmt.Errorf("decode release: %w", err)
	}
	return Release{
		Tag:       payload.TagName,
		Notes:     payload.Body,
		URL:       payload.HTMLURL,
		Published: payload.PublishedAt,
	}, nil
}

// RequestInstall drops the trigger file the privileged helper watches for.
func (s *Service) RequestInstall() error {
	if !s.Installable() {
		return fmt.Errorf("the update helper is not installed")
	}
	path := filepath.Join(s.stateDir, triggerName)
	if err := os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil {
		return fmt.Errorf("request update: %w", err)
	}
	return nil
}

// newer reports whether tag is a later version than current. Both look like
// "v1.2.3"; anything unparsable is treated as "not newer" so a malformed tag
// never nags the user into an update.
func newer(tag, current string) bool {
	a, ok := parse(tag)
	if !ok {
		return false
	}
	b, ok := parse(current)
	if !ok {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	// Ignore any pre-release or build suffix: only the numbers order releases.
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
