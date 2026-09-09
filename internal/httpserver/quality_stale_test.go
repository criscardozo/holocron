package httpserver

import (
	"strings"
	"testing"
	"time"

	"github.com/cristian/holocron/internal/settings"
)

// TestHumanAgeCannotBeSkimmedPast. A timestamp four days old reads as fine at a
// glance; "hace 4 días" is the same fact and does not. That difference is the
// whole point on a page whose numbers people delete things over.
func TestHumanAgeCannotBeSkimmedPast(t *testing.T) {
	t.Parallel()
	cases := map[time.Duration]string{
		30 * time.Second:     "recién",
		20 * time.Minute:     "hace 20 minutos",
		90 * time.Minute:     "hace una hora",
		5 * time.Hour:        "hace 5 horas",
		30 * time.Hour:       "hace un día",
		4 * 24 * time.Hour:   "hace 4 días",
		400 * 24 * time.Hour: "hace 400 días",
	}
	for d, want := range cases {
		if got := humanAge(d); got != want {
			t.Errorf("humanAge(%s) = %q, want %q", d, got, want)
		}
	}
}

// TestAReportFromAnotherVersionSaysSo is the failure ObiWan hit: after the
// release that stopped counting unaired episodes as ghosts, the page went on
// serving the previous report — same labels, old meanings — and the number it
// showed was one somebody might act on.
func TestAReportFromAnotherVersionSaysSo(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	// The page only renders a report when Jellyfin is linked, so the fixture
	// has to look linked.
	for k, v := range map[string]string{
		settings.KeyJellyfinURL:   "http://192.168.0.2:8096",
		settings.KeyJellyfinToken: "t",
	} {
		if err := ts.deps.Settings.Set(t.Context(), k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := ts.deps.Quality.SaveReportForTest(t.Context(), "0.10.1", time.Now()); err != nil {
		t.Fatal(err)
	}

	body := ts.get(t, "/quality", nil).Body
	if !strings.Contains(body, "0.10.1") {
		t.Error("the page does not name the version that produced the report")
	}
	if !strings.Contains(body, "Volvé a analizar") {
		t.Error("the page does not tell the user the numbers are not comparable")
	}
}
