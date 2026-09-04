package diskusage

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/db"
	"github.com/cristian/holocron/internal/folders"
	"github.com/cristian/holocron/internal/jobs"
)

// The media disk is exFAT written to from a Mac, so it carries .Spotlight-V100
// and .Trashes at the top level. Nobody asking what fills a media drive wants
// to see those. They are cheap to walk, so skipping them buys tidiness rather
// than speed.
func TestChildDirsSkipsHiddenDirectories(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, name := range []string{
		"Peliculas", "Series", "Museum",
		".Spotlight-V100", ".Trashes", ".fseventsd",
	} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Files at the top level are not scan targets either.
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := childDirs(root)
	if len(got) != 3 {
		t.Fatalf("got %d children, want 3: %v", len(got), got)
	}
	for _, dir := range got {
		if name := filepath.Base(dir); name[0] == '.' {
			t.Errorf("hidden directory %q should have been skipped", name)
		}
	}
	for _, want := range []string{"Peliculas", "Series", "Museum"} {
		if !slices.ContainsFunc(got, func(p string) bool { return filepath.Base(p) == want }) {
			t.Errorf("missing %q in %v", want, got)
		}
	}
}

func TestChildDirsOnAnUnreadableRoot(t *testing.T) {
	t.Parallel()
	// The caller falls back to scanning the root itself, so nil is the contract.
	if got := childDirs(filepath.Join(t.TempDir(), "missing")); got != nil {
		t.Errorf("childDirs = %v, want nil", got)
	}
}

func write(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newService(t *testing.T) (*Service, *folders.Store) {
	t.Helper()
	database, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	store := folders.NewStore(database)
	return NewService(database, store, jobs.NewManager()), store
}

func waitForScan(t *testing.T, svc *Service, folderID int64) jobs.Job {
	t.Helper()
	for range 400 {
		if job, ok := svc.LastJob(folderID); ok && job.Status != jobs.StatusRunning {
			if job.Status == jobs.StatusError {
				t.Fatalf("scan failed: %s", job.Err)
			}
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the scan did not finish")
	return jobs.Job{}
}

// TestScanCachesWhatItMeasured covers the whole loop the disk page depends on:
// walk, store, read back. The cache is what makes the page instant on load —
// nothing rescans on a page view.
func TestScanCachesWhatItMeasured(t *testing.T) {
	t.Parallel()
	svc, store := newService(t)

	root := t.TempDir()
	for name, size := range map[string]int{"Peliculas": 40960, "Series": 8192} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(dir, "file.bin"), size)
	}

	id, err := store.Add(t.Context(), "Media", root, folders.PurposeDisk)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, ok, err := svc.CachedResult(t.Context(), id); err != nil || ok {
		t.Fatalf("there is nothing cached before the first scan: ok=%v err=%v", ok, err)
	}

	if _, err := svc.StartScan(t.Context(), id); err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	waitForScan(t, svc, id)

	result, scannedAt, ok, err := svc.CachedResult(t.Context(), id)
	if err != nil || !ok {
		t.Fatalf("CachedResult = %v, %v", ok, err)
	}
	if scannedAt == "" {
		t.Error("the page shows when the scan ran")
	}
	if len(result.TopFolders) != 2 {
		t.Fatalf("got %d top folders, want 2: %+v", len(result.TopFolders), result.TopFolders)
	}
	// Ordered biggest first: the point of the list is what is filling the disk.
	if result.TopFolders[0].Name != "Peliculas" {
		t.Errorf("largest folder = %q, want Peliculas", result.TopFolders[0].Name)
	}
	if result.TopFolders[0].Bytes < 40960 {
		t.Errorf("measured %d bytes, want at least the file size", result.TopFolders[0].Bytes)
	}

	// A second scan replaces the row rather than adding one.
	if _, err := svc.StartScan(t.Context(), id); err != nil {
		t.Fatalf("second StartScan: %v", err)
	}
	waitForScan(t, svc, id)
	var rows int
	if err := svc.db.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM scan_results WHERE folder_id = ?`, id).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("%d cached rows for one folder, want 1", rows)
	}
}

// TestBrowseStaysInsideTheWatchedFolder is the security-relevant one: the path
// comes from the query string, and os.Root is what keeps it from walking out.
func TestBrowseStaysInsideTheWatchedFolder(t *testing.T) {
	t.Parallel()
	svc, store := newService(t)

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Peliculas"), 0o750); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "Peliculas", "dune.mkv"), 2048)

	outside := t.TempDir()
	write(t, filepath.Join(outside, "secret.txt"), 16)
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}

	id, err := store.Add(t.Context(), "Media", root, folders.PurposeDisk)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Browse(t.Context(), id, filepath.Join(root, "Peliculas")); err != nil {
		t.Fatalf("browsing a real subfolder should work: %v", err)
	}
	// The empty path means the watched folder itself, which is how the page
	// opens.
	if _, err := svc.Browse(t.Context(), id, ""); err != nil {
		t.Fatalf("browsing the root should work: %v", err)
	}

	for _, path := range []string{
		outside,                                // straight out
		filepath.Join(root, "..", "..", "etc"), // traversal through the root
		filepath.Join(root, "escape"),          // a symlink pointing out
		"/etc",
	} {
		if _, err := svc.Browse(t.Context(), id, path); err == nil {
			t.Errorf("Browse(%q) escaped the watched folder", path)
		}
	}
}

func TestScanOnAFolderThatIsNotThere(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t)
	// An id that was never added: the handler passes whatever is in the URL.
	if _, err := svc.StartScan(t.Context(), 999); err == nil {
		t.Error("scanning an unknown folder should fail")
	}
}
