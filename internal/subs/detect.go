// Package subs detects subtitle files that sit alongside media, and classifies
// whether a Spanish subtitle is present. It is shared by the .nfo generator
// (Phase 3) and the OpenSubtitles feature (Phase 4).
package subs

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// subtitleExts are the external subtitle file extensions we recognise.
var subtitleExts = map[string]bool{
	".srt": true,
	".ssa": true,
	".ass": true,
	".sub": true,
	".vtt": true,
}

// spanishMarkers are case-insensitive substrings that flag a Spanish subtitle.
var spanishMarkers = []string{
	".es.", ".spa.", ".esp.", ".es-", ".spa-", "-es.", "_es.",
	"spanish", "espanol", "español", "castellano", "latino",
}

// Result summarises the subtitle files found for a media item.
type Result struct {
	AnySubtitle     bool
	SpanishSubtitle bool
}

// DetectDir walks folder (bounded, skipping symlinks) looking for external
// subtitle files and reports whether any subtitle and a Spanish subtitle exist.
// A folder that cannot be read yields a zero Result and no error.
func DetectDir(folder string) Result {
	var res Result
	_ = filepath.WalkDir(folder, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d == nil {
			return nil //nolint:nilerr // best-effort scan
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !subtitleExts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		res.AnySubtitle = true
		if isSpanish(filepath.Base(path)) {
			res.SpanishSubtitle = true
		}
		return nil
	})
	return res
}

func isSpanish(name string) bool {
	lower := strings.ToLower(name)
	for _, m := range spanishMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}
