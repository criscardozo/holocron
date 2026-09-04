package naming

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ok   bool
	}{
		{"The Matrix (1999)", true},
		{"Blade Runner 2049 (2017)", true},
		{"Dune (2021) {edition-IMAX}", true},
		{"The Matrix", false},
		{"The.Matrix.1999.1080p", false},
		{"Interstellar 2014", false},
		{"Firefly (2002)", true},
		{"", false},
	}
	for _, c := range cases {
		if ok, _ := Validate(c.name); ok != c.ok {
			t.Errorf("Validate(%q) ok = %v, want %v", c.name, ok, c.ok)
		}
	}
}

func TestValidateSuggestsCorrection(t *testing.T) {
	t.Parallel()
	if _, expected := Validate("The.Matrix.1999.1080p"); expected != "The.Matrix..1080p (1999)" && expected != "The Matrix 1080p (1999)" {
		// The exact suggestion is best-effort; just require the year is placed
		// in parentheses at the end.
		if !hasYearSuffix(expected, "1999") {
			t.Errorf("expected suggestion ending in (1999), got %q", expected)
		}
	}
}

func hasYearSuffix(s, year string) bool {
	return len(s) >= len("("+year+")") && s[len(s)-len("("+year+")"):] == "("+year+")"
}

// TestScanDirIgnoresWhatIsNotAWatchableFolder: the scan reports folders, and a
// file or a symlink beside them is not one. A symlink in particular could point
// anywhere, and following it would report paths outside the library.
func TestScanDirIgnoresWhatIsNotAWatchableFolder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "The Matrix"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "Dune (2021)"), 0o750); err != nil {
		t.Fatal(err)
	}
	// A stray file with a bad name: not a folder, not an issue.
	if err := os.WriteFile(filepath.Join(root, "cover.jpg"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "elsewhere")); err != nil {
		t.Fatal(err)
	}

	issues, err := ScanDir(root, "movies")
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1: %+v", len(issues), issues)
	}
	if issues[0].Found != "The Matrix" {
		t.Errorf("reported %q", issues[0].Found)
	}
	if issues[0].Type != "movies" || issues[0].Path != filepath.Join(root, "The Matrix") {
		t.Errorf("unexpected issue: %+v", issues[0])
	}
}

func TestScanDirOnAMissingRoot(t *testing.T) {
	t.Parallel()
	// A watched folder can be removed or a disk unmounted. That is an error to
	// report, not an empty result that reads as "everything is fine".
	if _, err := ScanDir(filepath.Join(t.TempDir(), "gone"), "movies"); err == nil {
		t.Error("scanning a missing root should fail")
	}
}

func TestValidateEdgeCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ok   bool
		why  string
	}{
		{"Blade Runner 2049 (2017)", true, "a year inside the title as well as the suffix"},
		{"Se7en (1995)", true, "digits inside the title"},
		{"Dune: Parte Dos (2024)", true, "punctuation in the title"},
		{"El Padrino (Parte II) (1974)", true, "parentheses inside the title"},
		{"Amélie (2001)", true, "accented characters"},
		{"The Matrix (1999) - 1080p", false, "a suffix after the year breaks the pattern"},
		{"The Matrix (99)", false, "two digits are not a year"},
		{"The Matrix (1899)", false, "before cinema; the pattern only accepts 19xx/20xx"},
		{"  The Matrix (1999)", false, "leading whitespace"},
		{"The Matrix ( 1999 )", false, "spaces inside the parentheses"},
	}
	for _, c := range cases {
		if ok, _ := Validate(c.name); ok != c.ok {
			t.Errorf("Validate(%q) = %v, want %v — %s", c.name, ok, c.ok, c.why)
		}
	}
}

// TestSuggestionAlwaysNamesSomething: the suggestion is shown verbatim in the
// UI next to the offending folder, so it can never come out empty or as a bare
// year.
func TestSuggestionAlwaysNamesSomething(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"The.Matrix.1999.1080p", "1999", "[2021]", "___", "Sin año", "",
	} {
		ok, expected := Validate(name)
		if ok {
			continue
		}
		if strings.TrimSpace(expected) == "" {
			t.Errorf("Validate(%q) suggested nothing", name)
		}
		if !strings.HasSuffix(expected, ")") {
			t.Errorf("Validate(%q) = %q, want it to end in a parenthesised year", name, expected)
		}
	}
}
