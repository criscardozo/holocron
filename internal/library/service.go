// Package library syncs the movie/series inventory from Jellyfin into SQLite.
// Jellyfin reports each file's subtitle tracks, so nothing here touches the
// media disk to work out what is present.
//
// It used to write .nfo files too. Jellyfin writes those itself when a library
// has "save metadata as NFO" on, and two writers over one file means the last
// one wins — so Holocron stopped being one of them.
package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/cristian/holocron/internal/jellyfin"
	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/settings"
	"github.com/cristian/holocron/internal/version"
)

// KindSync is the job kind for an inventory sync.
const KindSync = "media-sync"

// ErrNotConfigured means the Jellyfin address or token has not been set. It is
// jellyfin.ErrNotLinked, so a handler comparing against either matches.
var ErrNotConfigured = jellyfin.ErrNotLinked

// Service manages the media inventory.
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
}

// Stats summarises the inventory.
type Stats struct {
	Total       int
	Movies      int
	WithoutSubs int
}

// Configured reports whether a Jellyfin connection has been established.
func (s *Service) Configured(ctx context.Context) bool {
	return jellyfin.Linked(ctx, s.settings)
}

func (s *Service) client(ctx context.Context) (*jellyfin.Client, error) {
	return jellyfin.FromSettings(ctx, s.settings, version.Current())
}

// TestConnection asks the server to identify itself, which needs the token and
// so verifies the whole configuration.
func (s *Service) TestConnection(ctx context.Context) (jellyfin.ServerInfo, error) {
	c, err := s.client(ctx)
	if err != nil {
		return jellyfin.ServerInfo{}, err
	}
	return c.Info(ctx)
}

// Reachable checks the address on its own, before there is a token. Without it
// the test button fails identically whether the address is wrong or the server
// simply has not been linked yet, which is the harder of the two to guess.
func (s *Service) Reachable(ctx context.Context) (jellyfin.ServerInfo, error) {
	base := s.settings.GetDefault(ctx, settings.KeyJellyfinURL, "")
	if base == "" {
		return jellyfin.ServerInfo{}, jellyfin.ErrNoServerURL
	}
	device := s.settings.GetDefault(ctx, settings.KeyJellyfinDeviceID, "")
	return jellyfin.New(base, "", device, version.Current()).PublicInfo(ctx)
}

// Syncing reports whether a sync is currently running.
func (s *Service) Syncing() bool { return s.jobs.IsRunning(KindSync) }

// LastJob returns the most recent sync job.
func (s *Service) LastJob() (jobs.Job, bool) { return s.jobs.Latest(KindSync) }

// StartSync fetches the movie/series inventory from Jellyfin into media_items.
func (s *Service) StartSync(ctx context.Context) (jobs.Job, error) {
	c, err := s.client(ctx)
	if err != nil {
		return jobs.Job{}, err
	}
	return s.jobs.Start(KindSync, func(jobCtx context.Context, p *jobs.Progress) (string, error) {
		return s.sync(jobCtx, c, p)
	})
}

func (s *Service) sync(ctx context.Context, c *jellyfin.Client, p *jobs.Progress) (string, error) {
	items, err := c.Items(ctx)
	if err != nil {
		return "", fmt.Errorf("list items: %w", err)
	}
	p.Set(60)

	// Collect first, write second: the transaction must not span the network
	// call above, because with a single database connection it would block
	// every other query for the length of the sync.
	type row struct {
		path, typ, title, itemID, providerIDs string
		year                                  int
		hasSubs                               bool
	}
	pending := make([]row, 0, len(items))
	skipped := 0
	for _, it := range items {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		// Jellyfin lists titles it knows from metadata providers but has never
		// seen on disk. There is nothing to inventory for those.
		if !it.OnDisk() {
			skipped++
			continue
		}
		folder := it.Folder()
		if folder == "" {
			skipped++
			continue
		}
		ids, err := json.Marshal(it.ProviderIDs)
		if err != nil {
			ids = []byte("{}")
		}
		pending = append(pending, row{
			path:  folder,
			typ:   mediaType(it.Type),
			title: it.Name,
			// Subtitles come from Jellyfin, which already knows every track of
			// every file — including embedded ones. This replaces walking each
			// title's directory, once per title, on a slow USB disk.
			hasSubs:     it.HasSpanishSubtitles(),
			itemID:      it.ID,
			providerIDs: string(ids),
			year:        it.Year,
		})
	}

	p.Set(80)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO media_items (path, type, title, year, server_item_id, has_subs_es, provider_ids)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(path) DO UPDATE SET
		   type = excluded.type, title = excluded.title, year = excluded.year,
		   server_item_id = excluded.server_item_id, has_subs_es = excluded.has_subs_es,
		   provider_ids = excluded.provider_ids`)
	if err != nil {
		return "", fmt.Errorf("prepare upsert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, r := range pending {
		if _, err := stmt.ExecContext(ctx,
			r.path, r.typ, r.title, r.year, r.itemID, boolToInt(r.hasSubs), r.providerIDs); err != nil {
			return "", fmt.Errorf("upsert media item: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit sync: %w", err)
	}

	if skipped > 0 {
		return fmt.Sprintf("%d ítems (%d sin archivo)", len(pending), skipped), nil
	}
	return fmt.Sprintf("%d ítems", len(pending)), nil
}

// mediaType maps Jellyfin's item types onto the values already stored in
// media_items, so existing rows and new ones agree.
func mediaType(t string) string {
	if t == jellyfin.TypeSeries {
		return "show"
	}
	return "movie"
}

// Stats returns inventory counters.
func (s *Service) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN type = 'movie' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN has_subs_es = 0 THEN 1 ELSE 0 END), 0)
		FROM media_items`).Scan(&st.Total, &st.Movies, &st.WithoutSubs)
	if err != nil {
		return Stats{}, fmt.Errorf("media stats: %w", err)
	}
	return st, nil
}

// Items lists inventory rows for display, ordered by type then title.
func (s *Service) Items(ctx context.Context, limit int) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, type, title, year, has_subs_es
		 FROM media_items ORDER BY type, title LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.Path, &it.Type, &it.Title, &it.Year, &it.HasSubsES); err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
