// Package library syncs the movie/series inventory from Jellyfin into SQLite.
// Jellyfin reports each file's subtitle tracks, so nothing here touches the
// media disk to work out what is present.
package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cristian/holocron/internal/jellyfin"
	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/nfo"
	"github.com/cristian/holocron/internal/settings"
	"github.com/cristian/holocron/internal/version"
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

// Configured reports whether a Jellyfin connection has been established.
func (s *Service) Configured(ctx context.Context) bool {
	return s.settings.GetDefault(ctx, settings.KeyJellyfinURL, "") != "" &&
		s.settings.GetDefault(ctx, settings.KeyJellyfinToken, "") != ""
}

func (s *Service) client(ctx context.Context) (*jellyfin.Client, error) {
	url := s.settings.GetDefault(ctx, settings.KeyJellyfinURL, "")
	token := s.settings.GetDefault(ctx, settings.KeyJellyfinToken, "")
	if url == "" || token == "" {
		return nil, ErrNotConfigured
	}
	device := s.settings.GetDefault(ctx, settings.KeyJellyfinDeviceID, "")
	return jellyfin.New(url, token, device, version.Current()), nil
}

// TestConnection asks the server to identify itself, to verify the connection.
func (s *Service) TestConnection(ctx context.Context) (jellyfin.ServerInfo, error) {
	c, err := s.client(ctx)
	if err != nil {
		return jellyfin.ServerInfo{}, err
	}
	return c.Info(ctx)
}

// Syncing reports whether a sync is currently running.
func (s *Service) Syncing() bool { return s.jobs.IsRunning(KindSync) }

// GeneratingNFO reports whether an .nfo generation is currently running.
func (s *Service) GeneratingNFO() bool { return s.jobs.IsRunning(KindNFO) }

// LastJob returns the most recent job of the given kind (KindSync or KindNFO).
func (s *Service) LastJob(kind string) (jobs.Job, bool) { return s.jobs.Latest(kind) }

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

// StartGenerateNFO writes a .nfo file for each inventory item whose folder is
// accessible on the host.
func (s *Service) StartGenerateNFO(ctx context.Context) (jobs.Job, error) {
	return s.jobs.Start(KindNFO, func(jobCtx context.Context, p *jobs.Progress) (string, error) {
		return s.generateNFO(jobCtx, p)
	})
}

func (s *Service) generateNFO(ctx context.Context, p *jobs.Progress) (string, error) {
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
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return "", fmt.Errorf("iterate media: %w", err)
	}
	if err := rows.Close(); err != nil {
		return "", fmt.Errorf("close media rows: %w", err)
	}

	var written, failed int
	var firstErr error
	for i, r := range items {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		p.Set(i * 100 / max(len(items), 1))

		ids := map[string]string{}
		if err := json.Unmarshal([]byte(r.altIDs), &ids); err != nil {
			ids = map[string]string{} // a malformed cache must not stop the job
		}
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
			// The folder may be unreachable from this host, or the service may
			// lack write access (systemd ProtectSystem=strict without the media
			// paths in ReadWritePaths). Keep going, but do not stay silent:
			// a run where everything fails must say so.
			failed++
			if firstErr == nil {
				firstErr = werr
			}
			continue
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE media_items SET nfo_written_at = datetime('now') WHERE path = ?`, r.path); err != nil {
			return "", fmt.Errorf("mark nfo written: %w", err)
		}
		written++
	}

	if written == 0 && failed > 0 {
		return "", fmt.Errorf("no se pudo escribir ningún .nfo (%d intentos): %w", failed, firstErr)
	}
	if failed > 0 {
		return fmt.Sprintf("%d .nfo escritos, %d fallaron", written, failed), nil
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

// Items lists inventory rows for display, ordered by type then title.
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

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
