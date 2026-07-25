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

// maxEntries bounds how many filesystem entries DetectDir will look at, so a
// pathological directory cannot stall a sync of the whole library.
const maxEntries = 5000

// DetectDir walks folder (skipping symlinks, stopping after maxEntries entries)
// looking for external subtitle files and reports whether any subtitle and a
// Spanish subtitle exist. It stops early once a Spanish subtitle is found.
// A folder that cannot be read yields a zero Result and no error.
func DetectDir(folder string) Result {
	var res Result
	seen := 0
	_ = filepath.WalkDir(folder, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d == nil {
			return nil //nolint:nilerr // best-effort scan
		}
		if seen++; seen > maxEntries {
			return filepath.SkipAll
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
			return filepath.SkipAll // nothing left to learn
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
