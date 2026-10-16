package httpserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cristian/holocron/web/templates"
)

// TestServiceWorkerCachesWhatTheOfflinePageLinks: the worker caches the
// offline page's assets by URL, and those URLs carry the release. If they
// drifted apart, the offline page would come up unstyled exactly when the
// server cannot be asked for the stylesheet.
func TestServiceWorkerCachesWhatTheOfflinePageLinks(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	sw := ts.get(t, "/sw.js", nil)
	if sw.Status != 200 || !strings.HasPrefix(sw.Header.Get("Content-Type"), "text/javascript") {
		t.Fatalf("sw.js: %d %q", sw.Status, sw.Header.Get("Content-Type"))
	}
	if got := sw.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("sw.js Cache-Control = %q: a kept worker holds the app on an old release", got)
	}
	if !strings.Contains(sw.Body, `const OFFLINE_ASSETS = ["`+templates.Asset("styles.css")) {
		t.Errorf("the worker does not cache this release's stylesheet:\n%.300s", sw.Body)
	}

	off := ts.get(t, "/offline", nil)
	if off.Status != 200 || !strings.Contains(off.Body, "Ginebra no responde") {
		t.Fatalf("offline page: %d", off.Status)
	}
	for _, a := range offlineAssets() {
		if strings.HasPrefix(a, "/static/holocron.svg") {
			continue
		}
		if !strings.Contains(off.Body, `"`+a+`"`) {
			t.Errorf("the offline page does not link %s, which is what the worker cached", a)
		}
	}
}

func TestManifestIsInstallable(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	r := ts.get(t, "/manifest.webmanifest", nil)
	if r.Status != 200 || r.Header.Get("Content-Type") != "application/manifest+json" {
		t.Fatalf("manifest: %d %q", r.Status, r.Header.Get("Content-Type"))
	}
	var m struct {
		StartURL string `json:"start_url"`
		Display  string `json:"display"`
		Icons    []struct {
			Src, Sizes, Purpose string
		} `json:"icons"`
	}
	if err := json.Unmarshal([]byte(r.Body), &m); err != nil {
		t.Fatalf("manifest is not JSON: %v", err)
	}
	if m.StartURL != "/" || m.Display != "standalone" {
		t.Errorf("start_url %q, display %q", m.StartURL, m.Display)
	}
	for _, ic := range m.Icons {
		if got := ts.get(t, ic.Src, nil).Status; got != 200 {
			t.Errorf("icon %s: %d", ic.Src, got)
		}
	}

	home := ts.get(t, "/", nil).Body
	for _, want := range []string{`rel="manifest"`, `viewport-fit=cover`, templates.Asset("app.js"), `popovertarget="mas"`,
		// The installed app is the server's: its name and its icon.
		`name="apple-mobile-web-app-title" content="Ginebra"`, `href="/static/ginebra-180.png"`} {
		if !strings.Contains(home, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
}
