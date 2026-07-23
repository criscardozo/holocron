// Package library syncs the movie/show inventory from Plex into SQLite and
// generates .nfo files for it. It ties together the Plex client, the subtitle
// detector and the .nfo writer.
package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/nfo"
	"github.com/cristian/holocron/internal/plex"
	"github.com/cristian/holocron/internal/settings"
	"github.com/cristian/holocron/internal/subs"
)

// Job kinds.
const (
	KindSync = "media-sync"
	KindNFO  = "nfo-generate"
)

// ErrNotConfigured means the Plex URL or token has not been set.
var ErrNotConfigured = errors.New("plex is not configured")

// Service manages the media inventory and .nfo generation.
type Service struct {
	db       *sql.DB
	settings *settings.Store
	jobs     *jobs.Manager
}

// NewService creates a Service.
func NewService(db *sql.DB, st *settings.Store, jm *jobs.Manager) *Service {
	return &Service{db: db, settings: st, jobs: jm}
}

// Item is a media inventory row for display.
type Item struct {
	Path      string
	Type      string
	Title     string
	Year      int
	HasSubsES bool
	HasNFO    bool
}

// Stats summarises the inventory.
type Stats struct {
	Total       int
	WithNFO     int
	WithoutSubs int
}

// Configured reports whether Plex credentials are set.
func (s *Service) Configured(ctx context.Context) bool {
	url := s.settings.GetDefault(ctx, settings.KeyPlexURL, "")
	token := s.settings.GetDefault(ctx, settings.KeyPlexToken, "")
	return url != "" && token != ""
}

func (s *Service) client(ctx context.Context) (*plex.Client, error) {
	url := s.settings.GetDefault(ctx, settings.KeyPlexURL, "")
	token := s.settings.GetDefault(ctx, settings.KeyPlexToken, "")
	if url == "" || token == "" {
		return nil, ErrNotConfigured
	}
	return plex.New(url, token), nil
}

// TestConnection lists the Plex libraries to verify the credentials work.
func (s *Service) TestConnection(ctx context.Context) ([]plex.Library, error) {
	c, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	return c.Libraries(ctx)
}

// Syncing reports whether a sync is currently running.
func (s *Service) Syncing() bool { return s.jobs.IsRunning(KindSync) }

// GeneratingNFO reports whether an .nfo generation is currently running.
func (s *Service) GeneratingNFO() bool { return s.jobs.IsRunning(KindNFO) }

// LastJob returns the most recent job of the given kind (KindSync or KindNFO).
func (s *Service) LastJob(kind string) (jobs.Job, bool) { return s.jobs.Latest(kind) }

// StartSync fetches the movie/show inventory from Plex into media_items.
func (s *Service) StartSync(ctx context.Context) (jobs.Job, error) {
	c, err := s.client(ctx)
	if err != nil {
		return jobs.Job{}, err
	}
	return s.jobs.Start(KindSync, func(jobCtx context.Context, p *jobs.Progress) (string, error) {
		return s.sync(jobCtx, c, p)
	})
}

func (s *Service) sync(ctx context.Context, c *plex.Client, p *jobs.Progress) (string, error) {
	libs, err := c.Libraries(ctx)
	if err != nil {
		return "", fmt.Errorf("list libraries: %w", err)
	}

	var count int
	for _, lib := range libs {
		if lib.Type != "movie" && lib.Type != "show" {
			continue
		}
		items, err := c.AllLibraryItems(ctx, lib.ID)
		if err != nil {
			return "", fmt.Errorf("list items of %q: %w", lib.Title, err)
		}
		for _, it := range items {
			folder := resolveFolder(it)
			if folder == "" {
				continue
			}
			detected := subs.DetectDir(folder)
			altIDs, _ := json.Marshal(parseAltIDs(it))
			if _, err := s.db.ExecContext(ctx,
				`INSERT INTO media_items (path, type, title, year, plex_guid, has_subs_es, plex_alt_ids)
				 VALUES (?, ?, ?, ?, ?, ?, ?)
				 ON CONFLICT(path) DO UPDATE SET
				   type = excluded.type, title = excluded.title, year = excluded.year,
				   plex_guid = excluded.plex_guid, has_subs_es = excluded.has_subs_es,
				   plex_alt_ids = excluded.plex_alt_ids`,
				folder, it.Type, it.Title, it.Year, it.Guid, boolToInt(detected.SpanishSubtitle), string(altIDs)); err != nil {
				return "", fmt.Errorf("upsert media item: %w", err)
			}
			count++
		}
	}
	return fmt.Sprintf("%d ítems", count), nil
}

// StartGenerateNFO writes a .nfo file for each inventory item whose folder is
// accessible on the host.
func (s *Service) StartGenerateNFO(ctx context.Context) (jobs.Job, error) {
	return s.jobs.Start(KindNFO, func(jobCtx context.Context, p *jobs.Progress) (string, error) {
		return s.generateNFO(jobCtx, p)
	})
}

func (s *Service) generateNFO(ctx context.Context, _ *jobs.Progress) (string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, type, title, year, plex_guid, has_subs_es, plex_alt_ids FROM media_items`)
	if err != nil {
		return "", fmt.Errorf("list media: %w", err)
	}
	type row struct {
		path, typ, title, guid, altIDs string
		year, hasSubs                  int
	}
	var items []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.path, &r.typ, &r.title, &r.year, &r.guid, &r.hasSubs, &r.altIDs); err != nil {
			_ = rows.Close()
			return "", fmt.Errorf("scan media: %w", err)
		}
		items = append(items, r)
	}
	if err := rows.Close(); err != nil {
		return "", err
	}

	var written int
	for _, r := range items {
		ids := map[string]string{}
		_ = json.Unmarshal([]byte(r.altIDs), &ids)
		it := nfo.Item{
			Title:          r.title,
			Year:           r.year,
			PlexGUID:       r.guid,
			IDs:            ids,
			HasSpanishSubs: r.hasSubs == 1,
		}
		var werr error
		switch r.typ {
		case "movie":
			_, werr = nfo.WriteMovie(r.path, it)
		case "show":
			_, werr = nfo.WriteShow(r.path, it)
		default:
			continue
		}
		if werr != nil {
			continue // folder not accessible on this host; skip
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE media_items SET nfo_written_at = datetime('now') WHERE path = ?`, r.path); err != nil {
			return "", fmt.Errorf("mark nfo written: %w", err)
		}
		written++
	}
	return fmt.Sprintf("%d .nfo escritos", written), nil
}

// Stats returns inventory counters.
func (s *Service) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN nfo_written_at IS NOT NULL THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN has_subs_es = 0 THEN 1 ELSE 0 END), 0)
		FROM media_items`).Scan(&st.Total, &st.WithNFO, &st.WithoutSubs)
	if err != nil {
		return Stats{}, fmt.Errorf("media stats: %w", err)
	}
	return st, nil
}

// Items lists inventory rows for display, newest-largest first by title.
func (s *Service) Items(ctx context.Context, limit int) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, type, title, year, has_subs_es, nfo_written_at IS NOT NULL
		 FROM media_items ORDER BY type, title LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.Path, &it.Type, &it.Title, &it.Year, &it.HasSubsES, &it.HasNFO); err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func resolveFolder(it plex.Metadata) string {
	switch it.Type {
	case "movie":
		if len(it.Media) > 0 && len(it.Media[0].Part) > 0 {
			return parentDir(it.Media[0].Part[0].File)
		}
	case "show":
		if len(it.Location) > 0 {
			return it.Location[0].Path
		}
	}
	return ""
}

// parentDir returns the directory of a file path, handling both / and \.
func parentDir(file string) string {
	file = strings.ReplaceAll(file, "\\", "/")
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		return file[:idx]
	}
	return ""
}

// parseAltIDs turns Plex's alternate GUIDs ("imdb://tt123") into a type→id map.
func parseAltIDs(it plex.Metadata) map[string]string {
	ids := map[string]string{}
	for _, g := range it.AltGUIDs {
		if typ, val, ok := strings.Cut(g.ID, "://"); ok {
			ids[typ] = val
		}
	}
	return ids
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
