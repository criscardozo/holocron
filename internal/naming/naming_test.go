package naming

import "testing"

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
