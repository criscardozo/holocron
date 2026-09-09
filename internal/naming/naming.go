// Package naming validates that media folders follow the "Title (Year)"
// convention Jellyfin and other scrapers expect.
package naming

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// A valid name is "Title (YYYY)" with a four-digit year in 1900-2099, optionally
// followed by edition tags like " {edition-Director's Cut}".
var (
	validRe = regexp.MustCompile(`^.+ \((?:19|20)\d{2}\)(?: \{[^}]+\})*$`)
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
	// Surrounding whitespace is invisible in most file managers and sorts the
	// folder somewhere unexpected, so it counts as a name to fix even when the
	// rest of the pattern holds. The suggestion below trims it.
	if name == strings.TrimSpace(name) && validRe.MatchString(name) {
		return true, ""
	}
	title, year, found := Parse(name)
	if title == "" {
		title = "Título"
	}
	if !found {
		return false, title + " (Año)"
	}
	suggestion := title + " (" + strconv.Itoa(year) + ")"
	if eds := Editions(name); len(eds) > 0 {
		suggestion += " " + strings.Join(eds, " ")
	}
	return false, suggestion
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
		if Hidden(e.Name()) {
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
