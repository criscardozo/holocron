package diskusage

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The media disk is exFAT written to from a Mac, so it carries .Spotlight-V100
// and .Trashes at the top level. Those are neither interesting to look at nor
// cheap to walk: .Spotlight-V100 holds many small entries, which is what makes
// a du-style scan slow.
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
