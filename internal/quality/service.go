package quality

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cristian/holocron/internal/jellyfin"
	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/settings"
	"github.com/cristian/holocron/internal/version"
)

// Errors a caller is expected to tell apart.
var (
	// ErrNotConfigured means Jellyfin has not been linked.
	ErrNotConfigured = jellyfin.ErrNotLinked
	// ErrNotAdmin means the linked account cannot ask the server to write
	// metadata. Reported before the request rather than after the 403.
	ErrNotAdmin = errors.New("the linked jellyfin account is not an administrator")
	// ErrUnknownItem means the item is not in the last report.
	ErrUnknownItem = errors.New("item is not in the current report")
	// ErrNoReport means no audit has been run yet.
	ErrNoReport = errors.New("no quality report yet")
)

// Service runs library audits and caches the last report.
type Service struct {
	db       *sql.DB
	settings *settings.Store
	jobs     *jobs.Manager
}

// NewService creates a Service.
func NewService(db *sql.DB, st *settings.Store, jm *jobs.Manager) *Service {
	return &Service{db: db, settings: st, jobs: jm}
}

// Configured reports whether there is a Jellyfin to audit.
func (s *Service) Configured(ctx context.Context) bool {
	return jellyfin.Linked(ctx, s.settings)
}

// Admin reports whether the linked account may ask for a metadata refresh.
func (s *Service) Admin(ctx context.Context) bool {
	return jellyfin.IsAdmin(ctx, s.settings)
}

// Scanning reports whether an audit is running.
func (s *Service) Scanning() bool { return s.jobs.IsRunning(Kind) }

// LastJob returns the most recent audit job.
func (s *Service) LastJob() (jobs.Job, bool) { return s.jobs.Latest(Kind) }

// StartScan audits the library in the background.
func (s *Service) StartScan(ctx context.Context) (jobs.Job, error) {
	c, err := jellyfin.FromSettings(ctx, s.settings, version.Current())
	if err != nil {
		return jobs.Job{}, err
	}
	return s.jobs.Start(Kind, func(jobCtx context.Context, p *jobs.Progress) (string, error) {
		return s.scan(jobCtx, c, p)
	})
}

func (s *Service) scan(ctx context.Context, c *jellyfin.Client, p *jobs.Progress) (string, error) {
	// The audit is the heaviest read Holocron makes of Jellyfin, and Jellyfin is
	// also what serves playback. Yield the CPU while it runs.
	restore := jobs.Deprioritise()
	defer restore()

	items, err := c.AuditItems(ctx, func(count, total int) {
		if total > 0 {
			p.Set(min(count*90/total, 90))
		}
	})
	if err != nil {
		return "", fmt.Errorf("audit items: %w", err)
	}

	report := Analyse(items)
	report.GeneratedAt = time.Now()
	p.Set(95)

	if err := s.save(ctx, report); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d ítems revisados, %d hallazgos", report.Scanned, report.Total()), nil
}

func (s *Service) save(ctx context.Context, report Report) error {
	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	// One row, replaced: only the latest report is ever shown, and an old one
	// would describe a library that has since changed.
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO quality_reports (id, generated_at, report) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   generated_at = excluded.generated_at, report = excluded.report`,
		report.GeneratedAt.UTC().Format(time.RFC3339), string(body)); err != nil {
		return fmt.Errorf("save report: %w", err)
	}
	return nil
}

// Latest returns the cached report. The second result is false when no audit
// has run yet, which is not an error.
func (s *Service) Latest(ctx context.Context) (Report, bool, error) {
	var body string
	err := s.db.QueryRowContext(ctx,
		`SELECT report FROM quality_reports WHERE id = 1`).Scan(&body)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Report{}, false, nil
	case err != nil:
		return Report{}, false, fmt.Errorf("read report: %w", err)
	}

	var report Report
	if err := json.Unmarshal([]byte(body), &report); err != nil {
		// Propagated rather than swallowed: the callers already treat an error
		// as "no report yet", which is the state the next scan fixes, and this
		// way the reason reaches the log instead of disappearing.
		return Report{}, false, fmt.Errorf("decode report: %w", err)
	}
	if report.Counts == nil {
		report.Counts = map[Category]int{}
	}
	return report, true, nil
}

// Refresh asks Jellyfin to re-read one item's metadata, which is what fixes a
// missing synopsis or a title the scraper never matched.
//
// The id is checked against the last report rather than trusted: it arrives
// from the browser and this ends in a write on the media server. Jellyfin also
// reaches out to the metadata provider on a full refresh, so this stays one
// item per click — there is deliberately no "fix all".
func (s *Service) Refresh(ctx context.Context, itemID string) error {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return ErrUnknownItem
	}
	if !s.Admin(ctx) {
		return ErrNotAdmin
	}
	report, ok, err := s.Latest(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoReport
	}
	if !report.Mentions(itemID) {
		return ErrUnknownItem
	}

	c, err := jellyfin.FromSettings(ctx, s.settings, version.Current())
	if err != nil {
		return err
	}
	if err := c.RefreshItem(ctx, itemID); err != nil {
		return fmt.Errorf("refresh item: %w", err)
	}
	return nil
}
