package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestScanReportsFolders(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sub := filepath.Join(root, "movies")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sub, "a.mkv"), 4096)
	writeFile(t, filepath.Join(root, "loose.txt"), 1024)

	sc := New(Options{Paths: []string{root}, TopLimit: 10, MaxReportedErrors: 10})
	res, err := sc.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(res.TopFolders) != 1 {
		t.Fatalf("TopFolders = %d, want 1", len(res.TopFolders))
	}
	if res.TopFolders[0].Bytes == 0 {
		t.Errorf("folder bytes = 0, want > 0")
	}
}

func TestBrowseListsChildren(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "season1"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "info.nfo"), 512)

	sc := New(Options{Paths: []string{root}})
	res, err := sc.Browse(context.Background(), root, 0)
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(res.Entries))
	}
	if res.Parent != "" {
		t.Errorf("parent = %q, want empty at scan root", res.Parent)
	}
}

func TestBrowseRejectsPathOutsideRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir() // a sibling temp dir, not under root

	sc := New(Options{Paths: []string{root}})
	if _, err := sc.Browse(context.Background(), outside, 0); err == nil {
		t.Fatal("expected error browsing outside the configured root, got nil")
	}
}

func TestBrowseRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	secret := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	// Browsing through a symlink that leaves the root must be rejected by the
	// os.Root containment check.
	sc := New(Options{Paths: []string{root}})
	if _, err := sc.Browse(context.Background(), link, 0); err == nil {
		t.Fatal("expected error browsing through an escaping symlink, got nil")
	}
}
