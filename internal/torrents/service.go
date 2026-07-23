// Package torrents manages downloads through a qBittorrent WebUI.
package torrents

import (
	"context"
	"errors"
	"strings"

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
	return qbittorrent.New(base,
		s.settings.GetDefault(ctx, settings.KeyQbitUser, ""),
		s.settings.GetDefault(ctx, settings.KeyQbitPass, ""))
}

// List returns all torrents.
func (s *Service) List(ctx context.Context) ([]qbittorrent.Torrent, error) {
	c, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	return c.Torrents(ctx)
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
