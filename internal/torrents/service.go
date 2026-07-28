// Package torrents manages downloads through a qBittorrent WebUI.
package torrents

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/cristian/holocron/internal/qbittorrent"
	"github.com/cristian/holocron/internal/settings"
)

// ErrNotConfigured means the qBittorrent URL has not been set.
var ErrNotConfigured = errors.New("qbittorrent is not configured")

// ErrInvalidMagnet means the provided string is not a magnet link.
var ErrInvalidMagnet = errors.New("not a magnet link")

// Service wraps a qBittorrent client configured from settings.
type Service struct {
	settings *settings.Store

	// The client is cached because it owns the session cookie. Building a new
	// one per call meant logging in again on every request, and the torrents
	// page refreshes every few seconds. It is rebuilt when the credentials
	// change.
	mu     sync.Mutex
	cached *qbittorrent.Client
	key    string
	cats   []qbittorrent.Category
	catsAt time.Time
}

// NewService creates a Service.
func NewService(st *settings.Store) *Service { return &Service{settings: st} }

// Summary is the dashboard view of torrent activity.
type Summary struct {
	Total   int
	Active  int
	DlSpeed int64
	UpSpeed int64
}

// Configured reports whether a qBittorrent URL is set.
func (s *Service) Configured(ctx context.Context) bool {
	return s.settings.GetDefault(ctx, settings.KeyQbitURL, "") != ""
}

func (s *Service) client(ctx context.Context) (*qbittorrent.Client, error) {
	base := s.settings.GetDefault(ctx, settings.KeyQbitURL, "")
	if base == "" {
		return nil, ErrNotConfigured
	}
	user := s.settings.GetDefault(ctx, settings.KeyQbitUser, "")
	pass := s.settings.GetDefault(ctx, settings.KeyQbitPass, "")
	key := base + "\x00" + user + "\x00" + pass

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached != nil && s.key == key {
		return s.cached, nil
	}
	c, err := qbittorrent.New(base, user, pass)
	if err != nil {
		return nil, err
	}
	// Pointing at a different server invalidates whatever was cached from the
	// old one.
	s.cached, s.key, s.cats, s.catsAt = c, key, nil, time.Time{}
	return c, nil
}

// List returns all torrents.
func (s *Service) List(ctx context.Context) ([]qbittorrent.Torrent, error) {
	c, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	return c.Torrents(ctx)
}

// categoriesTTL caches the category list. They change only when the user edits
// them in qBittorrent, while the torrents view is polled every few seconds —
// by the web page and by the iOS app.
const categoriesTTL = time.Minute

// Categories lists the categories configured in qBittorrent, so a magnet can be
// filed into one instead of landing in the default save path.
func (s *Service) Categories(ctx context.Context) ([]qbittorrent.Category, error) {
	s.mu.Lock()
	if s.cats != nil && time.Since(s.catsAt) < categoriesTTL {
		cached := s.cats
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	c, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	cats, err := c.Categories(ctx)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cats, s.catsAt = cats, time.Now()
	s.mu.Unlock()
	return cats, nil
}

// Summary aggregates torrent activity for the dashboard widget.
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	list, err := s.List(ctx)
	if err != nil {
		return Summary{}, err
	}
	sum := Summary{Total: len(list)}
	for _, t := range list {
		sum.DlSpeed += t.DlSpeed
		sum.UpSpeed += t.UpSpeed
		if t.DlSpeed > 0 || t.UpSpeed > 0 {
			sum.Active++
		}
	}
	return sum, nil
}

// Pause pauses a torrent.
func (s *Service) Pause(ctx context.Context, hash string) error {
	return s.act(ctx, func(c *qbittorrent.Client) error { return c.Pause(ctx, hash) })
}

// Resume resumes a torrent.
func (s *Service) Resume(ctx context.Context, hash string) error {
	return s.act(ctx, func(c *qbittorrent.Client) error { return c.Resume(ctx, hash) })
}

// Delete removes a torrent (without deleting its files).
func (s *Service) Delete(ctx context.Context, hash string) error {
	return s.act(ctx, func(c *qbittorrent.Client) error { return c.Delete(ctx, hash, false) })
}

// AddMagnet queues a magnet link after validating its scheme.
func (s *Service) AddMagnet(ctx context.Context, magnet, category string) error {
	magnet = strings.TrimSpace(magnet)
	if !strings.HasPrefix(magnet, "magnet:?") {
		return ErrInvalidMagnet
	}
	return s.act(ctx, func(c *qbittorrent.Client) error { return c.AddMagnet(ctx, magnet, category) })
}

func (s *Service) act(ctx context.Context, fn func(*qbittorrent.Client) error) error {
	c, err := s.client(ctx)
	if err != nil {
		return err
	}
	return fn(c)
}
