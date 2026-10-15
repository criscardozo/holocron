package httpserver

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestThemeIsPickedPerBrowser(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	// Nothing picked: the page follows the system, with both bar colours.
	home := ts.get(t, "/", nil).Body
	if !strings.Contains(home, `data-theme="auto"`) || !strings.Contains(home, `media="(prefers-color-scheme: light)"`) {
		t.Fatal("with no choice the page must follow the system")
	}

	r := ts.post(t, "/settings/theme", url.Values{"theme": {"light"}}, nil)
	if r.Status != http.StatusSeeOther {
		t.Fatalf("status %d", r.Status)
	}
	set := r.Header.Get("Set-Cookie")
	if !strings.Contains(set, themeCookie+"=light") || !strings.Contains(set, "HttpOnly") {
		t.Fatalf("Set-Cookie = %q", set)
	}

	light := ts.get(t, "/stack", map[string]string{"Cookie": themeCookie + "=light"}).Body
	for _, want := range []string{`data-theme="light"`, `content="default"`, `content="#f4f3f8"`} {
		if !strings.Contains(light, want) {
			t.Errorf("a light page lacks %s", want)
		}
	}
	settings := ts.get(t, "/settings", map[string]string{"Cookie": themeCookie + "=light"}).Body
	if !strings.Contains(settings, `value="light" class="theme-opt theme-opt-on" aria-pressed="true"`) {
		t.Error("Ajustes must show the picked scheme as pressed")
	}

	// Anything else is refused, and a forged cookie falls back to auto.
	if r := ts.post(t, "/settings/theme", url.Values{"theme": {"neon"}}, nil); r.Status != http.StatusBadRequest {
		t.Errorf("unknown theme: status %d", r.Status)
	}
	if b := ts.get(t, "/", map[string]string{"Cookie": themeCookie + "=\"><script>"}).Body; !strings.Contains(b, `data-theme="auto"`) {
		t.Error("a cookie that is not a theme must be ignored")
	}
}
