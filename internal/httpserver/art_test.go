package httpserver

import (
	"net/http"
	"strings"
	"testing"
)

func TestArtServesPostersAndNeverABrokenImage(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	got := ts.get(t, "/art/jf/"+testPosterID, map[string]string{"Accept-Encoding": "gzip"})
	if got.Status != 200 || got.Header.Get("Content-Type") != "image/jpeg" || got.Body != "jpeg" {
		t.Errorf("known poster: %d %q %q", got.Status, got.Header.Get("Content-Type"), got.Body)
	}
	if enc := got.Header.Get("Content-Encoding"); enc != "" {
		t.Errorf("a JPEG was sent with Content-Encoding %q", enc)
	}

	// Missing is a placeholder, so the page shows a blank card rather than
	// the browser's broken-image icon.
	missing := ts.get(t, "/art/jf/ffffffffffffffffffffffffffffffff", nil)
	if missing.Status != 200 || missing.Header.Get("Content-Type") != "image/svg+xml" {
		t.Errorf("missing poster: %d %q", missing.Status, missing.Header.Get("Content-Type"))
	}

	// Anything that is not an id is refused before it reaches a file name.
	for _, p := range []string{"/art/jf/nothex", "/art/plex/" + testPosterID, "/art/tmdb/..%2Fx.jpg"} {
		if r := ts.get(t, p, nil); r.Status != 404 {
			t.Errorf("%s: status %d, want 404", p, r.Status)
		}
	}
}

func TestTheStartPageIsTilesOverTheMural(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	if _, err := ts.db.Exec(`INSERT INTO media_items (path, type, title, year, server_item_id, has_subs_es, provider_ids)
		VALUES ('/m/Dune (2021)', 'movie', 'Dune', 2021, ?, 1, '{}')`, testPosterID); err != nil {
		t.Fatal(err)
	}

	body := ts.get(t, "/", nil).Body
	for _, want := range []string{
		`class="mural"`, "/art/jf/" + testPosterID, `hx-preserve="true"`,
		"/static/ginebra-marca.svg", `href="/activity"`, `href="/services"`, ">Torrents<",
		`class="app-card"`, `href="/stack"`,
		`class="side-label">Biblioteca<`, `aria-current="page"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the start page is missing %q", want)
		}
	}

	// A fragment swap has no layout to put a wall in.
	frag := ts.get(t, "/torrents/list", map[string]string{"HX-Request": "true"}).Body
	if strings.Contains(frag, `class="mural"`) {
		t.Error("an htmx fragment carried the mural")
	}
}

// TestTheStackPageSaysHowToReachEachApp: by name on the domain the page was
// asked for, so it works under holocron.merli.store and merli.store alike,
// and directly by the home-network address when there is one.
func TestTheStackPageSaysHowToReachEachApp(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	for _, host := range []string{"holocron.merli.store", "merli.store"} {
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/stack", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = host
		body := ts.do(t, req).Body
		for _, want := range []string{"https://jellyfin.merli.store/", "https://qb.merli.store/", "Seerr", "Lo que trabaja detrás"} {
			if !strings.Contains(body, want) {
				t.Errorf("Host %s: the page is missing %q", host, want)
			}
		}
		if strings.Contains(body, "holocron.merli.store") {
			t.Errorf("Host %s: a link carries Holocron's own host", host)
		}
	}
}
