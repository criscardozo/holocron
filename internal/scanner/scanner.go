// Package scanner computes on-demand disk usage for configured folders. It is
// ported from the sibling diskusage-pi project: sizes are allocated bytes
// (st_blocks * 512) like du, filesystem stats come from statfs, and the
// drill-down (Browse) is confined to configured roots.
//
// The containment check for user-supplied browse paths uses os.Root (Go 1.24+)
// instead of the original EvalSymlinks + prefix approach, so symlink and ".."
// escapes are rejected at the kernel.
//
// It relies on syscall.Statfs/Stat_t and is therefore Unix-only (Linux on the
// Pi, macOS on the dev machine).
package scanner

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Options configures a Scanner.
type Options struct {
	Paths             []string
	TopLimit          int
	ExcludePaths      []string
	OneFileSystem     bool
	MaxReportedErrors int
	Concurrency       int
}

// Scanner performs disk-usage scans.
type Scanner struct {
	options Options
}

// Result is a full scan of the configured paths.
type Result struct {
	GeneratedAt    time.Time     `json:"generatedAt"`
	ScanPaths      []string      `json:"scanPaths"`
	DiskPath       string        `json:"diskPath"`
	Hostname       string        `json:"hostname"`
	TotalBytes     uint64        `json:"totalBytes"`
	UsedBytes      uint64        `json:"usedBytes"`
	FreeBytes      uint64        `json:"freeBytes"`
	AvailableBytes uint64        `json:"availableBytes"`
	UsedPercent    float64       `json:"usedPercent"`
	TopFolders     []FolderUsage `json:"topFolders"`
	Errors         []ScanError   `json:"errors"`
	DurationMillis int64         `json:"durationMillis"`
}

// FolderUsage is the size of one top-level folder.
type FolderUsage struct {
	Path          string  `json:"path"`
	Name          string  `json:"name"`
	Bytes         uint64  `json:"bytes"`
	PercentOfDisk float64 `json:"percentOfDisk"`
}

// FolderEntry is one child in a Browse listing.
type FolderEntry struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Bytes uint64 `json:"bytes"`
	IsDir bool   `json:"isDir"`
}

// BrowseResult is one level of the drill-down.
type BrowseResult struct {
	Path           string        `json:"path"`
	Parent         string        `json:"parent,omitempty"`
	TotalBytes     uint64        `json:"totalBytes"`
	Entries        []FolderEntry `json:"entries"`
	Errors         []ScanError   `json:"errors"`
	DurationMillis int64         `json:"durationMillis"`
}

// ScanError is a non-fatal error encountered during a scan.
type ScanError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// New creates a Scanner.
func New(options Options) *Scanner {
	return &Scanner{options: options}
}

// FilesystemStat returns total/used/free/available bytes for the filesystem
// containing path. It is exported so callers (e.g. the disk widget) can show a
// folder's disk usage without a full recursive scan.
func FilesystemStat(path string) (total, used, free, available uint64, err error) {
	st, err := filesystemStat(path)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return st.total, st.used, st.free, st.available, nil
}

// Scan walks each configured path in parallel, sums allocated bytes, and
// returns the largest folders plus filesystem stats for the first existing
// path.
func (s *Scanner) Scan(ctx context.Context) (Result, error) {
	startedAt := time.Now()
	hostname, _ := os.Hostname()

	result := Result{
		GeneratedAt: startedAt.UTC(),
		ScanPaths:   append([]string(nil), s.options.Paths...),
		Hostname:    hostname,
		TopFolders:  []FolderUsage{},
		Errors:      []ScanError{},
	}
	scanErrors := errorCollector{limit: s.options.MaxReportedErrors, items: []ScanError{}}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	if diskPath := s.firstExistingPath(); diskPath != "" {
		stat, err := filesystemStat(diskPath)
		if err != nil {
			scanErrors.add(diskPath, err)
		} else {
			result.DiskPath = diskPath
			result.TotalBytes = stat.total
			result.FreeBytes = stat.free
			result.AvailableBytes = stat.available
			result.UsedBytes = stat.used
			if stat.total > 0 {
				result.UsedPercent = float64(stat.used) / float64(stat.total) * 100
			}
		}
	}

	concurrency := s.options.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	if len(s.options.Paths) > 0 && concurrency > len(s.options.Paths) {
		concurrency = len(s.options.Paths)
	}

	scanCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type folderResult struct {
		folder FolderUsage
		ok     bool
	}
	results := make([]folderResult, len(s.options.Paths))
	semaphore := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, path := range s.options.Paths {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case semaphore <- struct{}{}:
			case <-scanCtx.Done():
				return
			}
			defer func() { <-semaphore }()

			folder, ok := s.scanPath(scanCtx, path, &scanErrors)
			results[i] = folderResult{folder: folder, ok: ok}
		}()
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return result, err
	}

	folders := make([]FolderUsage, 0, len(results))
	for _, r := range results {
		if r.ok {
			folders = append(folders, r.folder)
		}
	}

	sort.Slice(folders, func(i, j int) bool { return folders[i].Bytes > folders[j].Bytes })
	if s.options.TopLimit > 0 && len(folders) > s.options.TopLimit {
		folders = folders[:s.options.TopLimit]
	}

	result.TopFolders = folders
	result.Errors = scanErrors.snapshot()
	result.DurationMillis = time.Since(startedAt).Milliseconds()
	return result, nil
}

// Browse lists the direct children of path with recursive sizes. path must live
// under one of the configured Paths; containment is enforced with os.Root.
func (s *Scanner) Browse(ctx context.Context, path string, limit int) (BrowseResult, error) {
	startedAt := time.Now()
	cleanPath, err := s.resolveBrowsePath(path)
	if err != nil {
		return BrowseResult{}, err
	}

	info, err := os.Lstat(cleanPath)
	if err != nil {
		return BrowseResult{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return BrowseResult{}, errors.New("path is a symlink")
	}
	if !info.IsDir() {
		return BrowseResult{}, errors.New("path is not a directory")
	}

	scanErrors := errorCollector{limit: s.options.MaxReportedErrors, items: []ScanError{}}

	dir, err := os.Open(cleanPath)
	if err != nil {
		return BrowseResult{}, err
	}
	dirEntries, readErr := dir.ReadDir(-1)
	_ = dir.Close()
	if readErr != nil {
		scanErrors.add(cleanPath, readErr)
	}

	rootDevice, hasRootDevice := deviceID(info)
	entries := make([]FolderEntry, 0, len(dirEntries))
	var totalBytes uint64

	for _, entry := range dirEntries {
		if err := ctx.Err(); err != nil {
			return BrowseResult{}, err
		}
		entryPath := filepath.Join(cleanPath, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if entry.IsDir() && s.shouldSkip(entryPath) {
			continue
		}

		var bytes uint64
		if entry.IsDir() {
			size, walkErr := s.directorySize(ctx, entryPath, rootDevice, hasRootDevice, &scanErrors)
			if walkErr != nil {
				return BrowseResult{}, walkErr
			}
			bytes = size
		} else {
			fi, err := entry.Info()
			if err != nil {
				scanErrors.add(entryPath, err)
				continue
			}
			bytes = allocatedBytes(fi)
		}

		entries = append(entries, FolderEntry{
			Path:  entryPath,
			Name:  entry.Name(),
			Bytes: bytes,
			IsDir: entry.IsDir(),
		})
		totalBytes += bytes
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Bytes > entries[j].Bytes })
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	parent := ""
	if !s.isAtScanRoot(cleanPath) {
		parent = filepath.Dir(cleanPath)
	}

	return BrowseResult{
		Path:           cleanPath,
		Parent:         parent,
		TotalBytes:     totalBytes,
		Entries:        entries,
		Errors:         scanErrors.snapshot(),
		DurationMillis: time.Since(startedAt).Milliseconds(),
	}, nil
}

// resolveBrowsePath cleans the requested path and verifies it lives under one
// of the configured Paths, using os.Root to reject symlink/".." escapes. The
// lexical clean path is returned so reported paths stay stable for the UI.
func (s *Scanner) resolveBrowsePath(requested string) (string, error) {
	if requested == "" {
		return "", errors.New("path is required")
	}
	abs, err := filepath.Abs(requested)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(abs)

	for _, root := range s.options.Paths {
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rootAbs, clean)
		if err != nil {
			continue
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue // lexically outside this root
		}

		// os.Root confines the lookup to rootAbs at the kernel level: a
		// symlink or ".." that escapes the root fails here.
		r, err := os.OpenRoot(rootAbs)
		if err != nil {
			continue
		}
		_, statErr := r.Stat(rel)
		_ = r.Close()
		if statErr != nil {
			return "", statErr
		}
		return clean, nil
	}
	return "", errors.New("path is outside configured scan paths")
}

func (s *Scanner) isAtScanRoot(path string) bool {
	for _, root := range s.options.Paths {
		if rootAbs, err := filepath.Abs(root); err == nil && filepath.Clean(rootAbs) == path {
			return true
		}
	}
	return false
}

func (s *Scanner) scanPath(ctx context.Context, path string, errs *errorCollector) (FolderUsage, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		errs.add(path, err)
		return FolderUsage{}, false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		errs.add(path, errors.New("configured path is a symlink"))
		return FolderUsage{}, false
	}
	if !info.IsDir() {
		errs.add(path, errors.New("configured path is not a directory"))
		return FolderUsage{}, false
	}

	rootDevice, hasRootDevice := deviceID(info)
	size, err := s.directorySize(ctx, path, rootDevice, hasRootDevice, errs)
	if err != nil {
		errs.add(path, err)
		return FolderUsage{}, false
	}

	folderStat, statErr := filesystemStat(path)
	if statErr != nil {
		errs.add(path, statErr)
	}

	return FolderUsage{
		Path:          path,
		Name:          folderName(path),
		Bytes:         size,
		PercentOfDisk: percent(size, folderStat.total),
	}, true
}

func (s *Scanner) directorySize(ctx context.Context, root string, rootDevice uint64, hasRootDevice bool, errCol *errorCollector) (uint64, error) {
	var total uint64

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			errCol.add(path, walkErr)
			return nil
		}
		if entry == nil {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if path != root && entry.IsDir() && s.shouldSkip(path) {
			return filepath.SkipDir
		}

		info, err := entry.Info()
		if err != nil {
			errCol.add(path, err)
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path != root && entry.IsDir() && s.options.OneFileSystem && hasRootDevice {
			if childDevice, ok := deviceID(info); ok && childDevice != rootDevice {
				return filepath.SkipDir
			}
		}

		total += allocatedBytes(info)
		return nil
	})
	if err != nil {
		return total, err
	}
	return total, nil
}

func (s *Scanner) firstExistingPath() string {
	for _, path := range s.options.Paths {
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			continue
		}
		return path
	}
	return ""
}

func (s *Scanner) shouldSkip(path string) bool {
	cleanPath := filepath.Clean(path)
	for _, excluded := range s.options.ExcludePaths {
		if cleanPath == excluded {
			return true
		}
		prefix := excluded
		if !strings.HasSuffix(prefix, string(os.PathSeparator)) {
			prefix += string(os.PathSeparator)
		}
		if strings.HasPrefix(cleanPath, prefix) {
			return true
		}
	}
	return false
}

type diskStat struct {
	total     uint64
	used      uint64
	free      uint64
	available uint64
}

func filesystemStat(path string) (diskStat, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return diskStat{}, err
	}
	blockSize := uint64(stat.Bsize)
	total := stat.Blocks * blockSize
	free := stat.Bfree * blockSize
	available := stat.Bavail * blockSize
	used := total - free
	return diskStat{total: total, used: used, free: free, available: available}, nil
}

func allocatedBytes(info fs.FileInfo) uint64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if ok && stat.Blocks > 0 {
		return uint64(stat.Blocks) * 512
	}
	if info.Size() <= 0 {
		return 0
	}
	return uint64(info.Size())
}

func deviceID(info fs.FileInfo) (uint64, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(stat.Dev), true
}

func percent(value, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(value) / float64(total) * 100
}

func folderName(path string) string {
	cleanPath := filepath.Clean(path)
	if cleanPath == string(os.PathSeparator) {
		return cleanPath
	}
	return filepath.Base(cleanPath)
}

type errorCollector struct {
	mu    sync.Mutex
	limit int
	items []ScanError
}

func (c *errorCollector) add(path string, err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.limit == 0 || len(c.items) >= c.limit {
		return
	}
	c.items = append(c.items, ScanError{Path: path, Message: err.Error()})
}

func (c *errorCollector) snapshot() []ScanError {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]ScanError, len(c.items))
	copy(out, c.items)
	return out
}
