package httpserver

import (
	"net/url"
	"strings"
	"testing"

	"github.com/cristian/holocron/internal/settings"
)

// TestAConfiguredCardStopsAskingForCredentials is the complaint this fixes: the
// card looked identical whether or not anything was saved, so there was no way
// to tell set up from not set up.
func TestAConfiguredCardStopsAskingForCredentials(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ctx := t.Context()

	body := ts.get(t, "/settings", nil).Body
	if !strings.Contains(body, `name="password"`) {
		t.Fatal("an unconfigured qBittorrent card should ask for a password")
	}

	for k, v := range map[string]string{
		settings.KeyQbitURL:  "http://127.0.0.1:8080",
		settings.KeyQbitUser: "admin",
		settings.KeyQbitPass: "secreto",
	} {
		if err := ts.deps.Settings.Set(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}

	body = ts.get(t, "/settings", nil).Body
	if !strings.Contains(body, "Ya conectado") {
		t.Error("a configured card should say so")
	}
	if !strings.Contains(body, "Borrar credenciales y reconectar") {
		t.Error("a configured card should offer to start over")
	}
	if strings.Contains(body, `placeholder="contraseña de la WebUI"`) {
		t.Error("a configured card is still asking for the password")
	}
	// What is stored is confirmed, never revealed.
	if strings.Contains(body, "secreto") {
		t.Error("the stored password was rendered into the page")
	}
	if !strings.Contains(body, "admin") || !strings.Contains(body, "127.0.0.1:8080") {
		t.Error("the card should confirm what it is configured with")
	}
}

// TestClearingCredentialsBringsTheFormBack, and clears every key rather than
// leaving a half-configured card behind.
func TestClearingCredentialsBringsTheFormBack(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ctx := t.Context()

	for k, v := range map[string]string{
		settings.KeyQbitURL:  "http://127.0.0.1:8080",
		settings.KeyQbitUser: "admin",
		settings.KeyQbitPass: "secreto",
	} {
		if err := ts.deps.Settings.Set(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}

	ts.post(t, "/settings/qbittorrent/clear", url.Values{}, nil)

	for _, k := range []string{settings.KeyQbitURL, settings.KeyQbitUser, settings.KeyQbitPass} {
		if _, ok, _ := ts.deps.Settings.Get(ctx, k); ok {
			t.Errorf("%s survived the clear", k)
		}
	}
	if body := ts.get(t, "/settings", nil).Body; !strings.Contains(body, `placeholder="contraseña de la WebUI"`) {
		t.Error("the form did not come back")
	}
}

// TestOpenSubtitlesHasNoLiveCheck. Deliberate: the only probe available spends
// a download from a small daily quota, and burning one to draw a badge is a bad
// trade. Pinned so nobody adds it without noticing the cost.
func TestOpenSubtitlesHasNoLiveCheck(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	if err := ts.deps.Settings.Set(t.Context(), settings.KeyOpenSubtitlesKey, "k"); err != nil {
		t.Fatal(err)
	}
	if body := ts.get(t, "/settings", nil).Body; strings.Contains(body, "/settings/status/opensubtitles") {
		t.Error("OpenSubtitles grew a live check")
	}
}

// TestTheManageScreenSaysWhereTheTokenComesFrom. Asking for a credential
// without saying where it lives turns a safety measure into a dead end.
func TestTheManageScreenSaysWhereTheTokenComesFrom(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	installHelper(t, ts)

	body := ts.get(t, "/manage", nil).Body
	if !strings.Contains(body, "¿De dónde saco el token?") {
		t.Error("the token field does not explain itself")
	}
	if !strings.Contains(body, "Ajustes → App iOS") {
		t.Error("the hint should point at the exact place")
	}
	if !strings.Contains(body, "Gestión de ObiWan") {
		t.Error("the page should say what it is")
	}
	// The icon is a <use href="#info"> against a sprite defined in the layout.
	// Both halves have to be in the same document or it renders as nothing at
	// all — silently, which is how a missing icon survives a screenshot.
	if !strings.Contains(body, `<use href="#info">`) {
		t.Error("the hint has no info icon")
	}
	if !strings.Contains(body, `<symbol id="info"`) {
		t.Error(`the sprite has no "info" symbol, so the icon renders as nothing`)
	}
}
