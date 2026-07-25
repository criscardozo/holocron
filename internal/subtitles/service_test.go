package subtitles

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSubtitleBaseName(t *testing.T) {
	t.Parallel()

	t.Run("uses the largest video file so players pair the subtitle", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		write(t, filepath.Join(dir, "Dune.Part.Two.2024.1080p.mkv"), 4096)
		write(t, filepath.Join(dir, "sample.mkv"), 16)
		write(t, filepath.Join(dir, "movie.nfo"), 32)

		if got, want := subtitleBaseName(dir), "Dune.Part.Two.2024.1080p"; got != want {
			t.Errorf("subtitleBaseName = %q, want %q", got, want)
		}
	})

	t.Run("falls back to the folder name when there is no video", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "Interstellar (2014)")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(dir, "movie.nfo"), 32)

		if got, want := subtitleBaseName(dir), "Interstellar (2014)"; got != want {
			t.Errorf("subtitleBaseName = %q, want %q", got, want)
		}
	})

	t.Run("falls back to the folder name when the folder is unreadable", func(t *testing.T) {
		t.Parallel()
		missing := filepath.Join(t.TempDir(), "does-not-exist")
		if got, want := subtitleBaseName(missing), "does-not-exist"; got != want {
			t.Errorf("subtitleBaseName = %q, want %q", got, want)
		}
	})
}
