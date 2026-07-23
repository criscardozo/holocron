// Package diskusage orchestrates disk scans for watched folders: it runs the
// scanner as a background job, caches the result in SQLite, and powers the
// drill-down.
package diskusage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/jobs"
	"github.com/cristian/holocron/internal/scanner"
)

// scan tuning kept modest for a low-powered device.
const (
	topLimit     = 50
	browseLimit  = 200
	maxScanError = 50
	concurrency  = 2
)

// Service coordinates scans, caching and browsing.
type Service struct {
	db      *sql.DB
	folders *folders.Store
	jobs    *jobs.Manager
}

// NewService creates a Service.
func NewService(db *sql.DB, fs *folders.Store, jm *jobs.Manager) *Service {
	return &Service{db: db, folders: fs, jobs: jm}
}

// JobKind returns the job kind for scanning a given folder.
func JobKind(folderID int64) string {
	return fmt.Sprintf("disk-scan-%d", folderID)
}

// StartScan launches a scan of the folder as a background job, storing the
// result in scan_results on completion.
func (s *Service) StartScan(ctx context.Context, folderID int64) (jobs.Job, error) {
	folder, err := s.folders.Get(ctx, folderID)
	if err != nil {
		return jobs.Job{}, err
	}

	// Scan the folder's immediate subdirectories so the result shows the
	// largest subfolders. Fall back to the folder itself if it has none.
	paths := childDirs(folder.Path)
	if len(paths) == 0 {
		paths = []string{folder.Path}
	}

	sc := scanner.New(scanner.Options{
		Paths:             paths,
		TopLimit:          topLimit,
		MaxReportedErrors: maxScanError,
		Concurrency:       concurrency,
	})

	return s.jobs.Start(JobKind(folderID), func(jobCtx context.Context, _ *jobs.Progress) (string, error) {
		result, err := sc.Scan(jobCtx)
		if err != nil {
			return "", fmt.Errorf("scan %s: %w", folder.Path, err)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return "", fmt.Errorf("encode result: %w", err)
		}
		if _, err := s.db.ExecContext(jobCtx,
			`INSERT INTO scan_results (folder_id, result, scanned_at)
			 VALUES (?, ?, datetime('now'))
			 ON CONFLICT(folder_id) DO UPDATE SET result = excluded.result, scanned_at = excluded.scanned_at`,
			folderID, string(encoded)); err != nil {
			return "", fmt.Errorf("store result: %w", err)
		}
		return fmt.Sprintf("%d folders", len(result.TopFolders)), nil
	})
}

// Scanning reports whether a scan of the folder is currently running.
func (s *Service) Scanning(folderID int64) bool {
	return s.jobs.IsRunning(JobKind(folderID))
}

// LastJob returns the most recent scan job for the folder, if any.
func (s *Service) LastJob(folderID int64) (jobs.Job, bool) {
	return s.jobs.Latest(JobKind(folderID))
}

// CachedResult returns the last stored scan for the folder, if any.
func (s *Service) CachedResult(ctx context.Context, folderID int64) (scanner.Result, string, bool, error) {
	var encoded, scannedAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT result, scanned_at FROM scan_results WHERE folder_id = ?`, folderID).
		Scan(&encoded, &scannedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return scanner.Result{}, "", false, nil
	}
	if err != nil {
		return scanner.Result{}, "", false, fmt.Errorf("read cached result: %w", err)
	}
	var result scanner.Result
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		return scanner.Result{}, "", false, fmt.Errorf("decode cached result: %w", err)
	}
	return result, scannedAt, true, nil
}

// childDirs returns the immediate subdirectories of root (skipping symlinks and
// files). Errors are ignored: the caller falls back to scanning root itself.
func childDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(root, e.Name()))
		}
	}
	return dirs
}

// Browse lists the direct children of path (confined to the folder root).
func (s *Service) Browse(ctx context.Context, folderID int64, path string) (scanner.BrowseResult, error) {
	folder, err := s.folders.Get(ctx, folderID)
	if err != nil {
		return scanner.BrowseResult{}, err
	}
	if path == "" {
		path = folder.Path
	}
	sc := scanner.New(scanner.Options{
		Paths:             []string{folder.Path},
		MaxReportedErrors: maxScanError,
	})
	return sc.Browse(ctx, path, browseLimit)
}
