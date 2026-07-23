// Package naming validates that media folders follow the "Title (Year)"
// convention used by Plex and other scrapers.
package naming

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A valid name is "Title (YYYY)" with a four-digit year in 1900-2099, optionally
// followed by edition tags like " {edition-Director's Cut}".
var (
	validRe = regexp.MustCompile(`^.+ \((?:19|20)\d{2}\)(?: \{[^}]+\})*$`)
	yearRe  = regexp.MustCompile(`(?:19|20)\d{2}`)
)

// Issue is a folder that violates the naming convention.
type Issue struct {
	Path     string // absolute path of the offending folder
	Type     string // movies | tv
	Found    string // the current folder name
	Expected string // a suggested corrected name
}

// Validate reports whether name follows the convention. When it does not, it
// returns a suggested corrected name (best-effort).
func Validate(name string) (ok bool, expected string) {
	if validRe.MatchString(name) {
		return true, ""
	}
	if year := yearRe.FindString(name); year != "" {
		title := strings.NewReplacer("("+year+")", "", "["+year+"]", "", year, "").Replace(name)
		title = strings.Trim(title, " .-_[]()")
		title = strings.Join(strings.Fields(title), " ")
		if title == "" {
			title = "Título"
		}
		return false, title + " (" + year + ")"
	}
	title := strings.Join(strings.Fields(strings.Trim(name, " .-_[]()")), " ")
	if title == "" {
		title = "Título"
	}
	return false, title + " (Año)"
}

// ScanDir validates the immediate subdirectories of root and returns the ones
// that break the convention. Symlinks and files are ignored.
func ScanDir(root, mediaType string) ([]Issue, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var issues []Issue
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 || !e.IsDir() {
			continue
		}
		if ok, expected := Validate(e.Name()); !ok {
			issues = append(issues, Issue{
				Path:     filepath.Join(root, e.Name()),
				Type:     mediaType,
				Found:    e.Name(),
				Expected: expected,
			})
		}
	}
	return issues, nil
}
