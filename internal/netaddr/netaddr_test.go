package netaddr

import (
	"errors"
	"testing"
)

// TestNormalise covers what people actually type into the address fields.
// "192.168.0.2:8096" is the case that broke a real install: net/url reads the
// colon as a scheme separator, so the request was never built and every call
// reported a generic "could not talk to Jellyfin".
func TestNormalise(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
		bad      bool
	}{
		{in: "192.168.0.2:8096", want: "http://192.168.0.2:8096"},
		{in: "  192.168.0.2:8096  ", want: "http://192.168.0.2:8096"},
		{in: "obiwan:8096", want: "http://obiwan:8096"},
		{in: "192.168.0.2", want: "http://192.168.0.2"},
		{in: "127.0.0.1:8080", want: "http://127.0.0.1:8080"},
		{in: "http://192.168.0.2:8096", want: "http://192.168.0.2:8096"},
		{in: "http://192.168.0.2:8096/", want: "http://192.168.0.2:8096"},
		{in: "HTTPS://Obiwan:8920", want: "https://Obiwan:8920"},
		// Behind a reverse proxy these services often live under a subpath.
		{in: "https://casa.example/jellyfin/", want: "https://casa.example/jellyfin"},
		// Credentials, query and fragment would repeat on every request.
		{in: "http://user:pass@obiwan:8096?x=1#f", want: "http://obiwan:8096"},
		// Empty means "not set", which is allowed.
		{in: "", want: ""},
		{in: "   ", want: ""},
		{in: "///", want: ""},
		// Not addresses.
		{in: "ftp://obiwan:8096", bad: true},
		{in: "http://", bad: true},
		{in: "file:///etc/passwd", bad: true},
	}
	for _, tc := range cases {
		got, err := Normalise(tc.in)
		if tc.bad {
			if !errors.Is(err, ErrBadAddress) {
				t.Errorf("Normalise(%q) = %q, %v; want ErrBadAddress", tc.in, got, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("Normalise(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Normalise(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestRepairIsForgiving checks the read path: it never rejects, because it runs
// against a value already in the database and refusing there would only turn a
// broken address into a broken page.
func TestRepairIsForgiving(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"192.168.0.2:8096":       "http://192.168.0.2:8096",
		"http://obiwan:8096/":    "http://obiwan:8096",
		"ftp://obiwan":           "ftp://obiwan",
		"":                       "",
		"   ":                    "",
		"https://casa.example/j": "https://casa.example/j",
	}
	for in, want := range cases {
		if got := Repair(in); got != want {
			t.Errorf("Repair(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestNormaliseIsIdempotent matters because Repair runs on every client build,
// over a value Normalise already wrote.
func TestNormaliseIsIdempotent(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"192.168.0.2:8096", "https://casa.example/jellyfin/", "obiwan"} {
		once, err := Normalise(in)
		if err != nil {
			t.Fatalf("Normalise(%q): %v", in, err)
		}
		twice, err := Normalise(once)
		if err != nil {
			t.Fatalf("Normalise(%q): %v", once, err)
		}
		if once != twice {
			t.Errorf("%q: %q then %q", in, once, twice)
		}
		if got := Repair(once); got != once {
			t.Errorf("Repair(%q) = %q, want it unchanged", once, got)
		}
	}
}
