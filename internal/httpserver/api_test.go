package httpserver

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/cristian/holocron/internal/settings"
)

func TestAPIRequiresABearerToken(t *testing.T) {
	t.Parallel()

	t.Run("no token configured on the server", func(t *testing.T) {
		t.Parallel()
		ts := newTestServer(t)
		// 503 rather than 401: the caller did nothing wrong, the server has no
		// token yet, and the app needs to tell those apart.
		resp := ts.get(t, "/api/v1/system", map[string]string{"Authorization": "Bearer whatever"})
		if resp.Status != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", resp.Status)
		}
		var payload map[string]string
		if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if payload["error"] == "" {
			t.Errorf("errors are always {\"error\": …}, got %v", payload)
		}
	})

	t.Run("with a token configured", func(t *testing.T) {
		t.Parallel()
		ts := newTestServer(t)
		token, err := ts.deps.APIToken.Generate(t.Context())
		if err != nil {
			t.Fatal(err)
		}

		cases := []struct {
			name, header string
			want         int
		}{
			{"missing header", "", http.StatusUnauthorized},
			{"not a bearer", "Basic " + token, http.StatusUnauthorized},
			{"wrong token", "Bearer " + strings.Repeat("a", len(token)), http.StatusUnauthorized},
			{"right token", "Bearer " + token, http.StatusOK},
		}
		for _, tc := range cases {
			headers := map[string]string{}
			if tc.header != "" {
				headers["Authorization"] = tc.header
			}
			resp := ts.get(t, "/api/v1/system", headers)
			if resp.Status != tc.want {
				t.Errorf("%s: status = %d, want %d", tc.name, resp.Status, tc.want)
			}
		}
	})

	t.Run("a revoked token stops working", func(t *testing.T) {
		t.Parallel()
		ts := newTestServer(t)
		token, err := ts.deps.APIToken.Generate(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err := ts.deps.APIToken.Revoke(t.Context()); err != nil {
			t.Fatal(err)
		}
		resp := ts.get(t, "/api/v1/system", map[string]string{"Authorization": "Bearer " + token})
		if resp.Status == http.StatusOK {
			t.Error("a revoked token was accepted")
		}
	})
}

// TestUpdateInstallNeedsTheAPIToken guards the one action in the web UI that
// replaces the binary and restarts the service. The UI is otherwise
// unauthenticated by design, so this gate is the whole protection.
func TestUpdateInstallNeedsTheAPIToken(t *testing.T) {
	t.Parallel()

	t.Run("no token pasted", func(t *testing.T) {
		t.Parallel()
		ts := newTestServer(t)
		if _, err := ts.deps.APIToken.Generate(t.Context()); err != nil {
			t.Fatal(err)
		}
		resp := ts.post(t, "/settings/updates/install", url.Values{"token": {""}}, nil)
		if got := resp.Body; !strings.Contains(got, "Pegá el token") {
			t.Errorf("expected the paste-the-token message, got %q", got)
		}
	})

	t.Run("wrong token is refused and logged", func(t *testing.T) {
		t.Parallel()
		ts := newTestServer(t)
		if _, err := ts.deps.APIToken.Generate(t.Context()); err != nil {
			t.Fatal(err)
		}
		resp := ts.post(t, "/settings/updates/install", url.Values{"token": {"nope"}}, nil)
		if got := resp.Body; !strings.Contains(got, "no coincide") {
			t.Errorf("expected the mismatch message, got %q", got)
		}
		if !strings.Contains(ts.logs.String(), "rejected update request") {
			t.Error("a wrong token here is either a typo or someone probing; it must be logged")
		}
	})

	t.Run("no token generated at all", func(t *testing.T) {
		t.Parallel()
		ts := newTestServer(t)
		resp := ts.post(t, "/settings/updates/install", url.Values{"token": {"anything"}}, nil)
		if got := resp.Body; !strings.Contains(got, "No hay token generado") {
			t.Errorf("expected the no-token-yet message, got %q", got)
		}
	})

	t.Run("right token reaches the helper", func(t *testing.T) {
		t.Parallel()
		ts := newTestServer(t)
		token, err := ts.deps.APIToken.Generate(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		resp := ts.post(t, "/settings/updates/install", url.Values{"token": {token}}, nil)
		// The helper is not installed in a test environment, so the request
		// fails at that point — which is proof it got past the token gate.
		got := resp.Body
		if strings.Contains(got, "no coincide") || strings.Contains(got, "Pegá el token") {
			t.Errorf("a valid token was refused: %q", got)
		}
	})
}

// TestServerAddressesAreNormalisedOnTheWayIn covers the bug that made Jellyfin
// unreachable: a bare host:port is not a URL, and stored as typed it made every
// later call fail with a generic "could not connect".
func TestServerAddressesAreNormalisedOnTheWayIn(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, path, field, key string
	}{
		{"jellyfin", "/settings/jellyfin", "url", settings.KeyJellyfinURL},
		{"qbittorrent", "/settings/qbittorrent", "url", settings.KeyQbitURL},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ts := newTestServer(t)

			ts.post(t, tc.path, url.Values{tc.field: {"192.168.0.2:8096"}}, nil)
			if got := ts.deps.Settings.GetDefault(t.Context(), tc.key, ""); got != "http://192.168.0.2:8096" {
				t.Errorf("stored %q, want the scheme added", got)
			}

			// A real typo is refused while the form is still open, and must not
			// overwrite what was there.
			resp := ts.post(t, tc.path, url.Values{tc.field: {"ftp://obiwan"}}, nil)
			if loc := resp.Header.Get("Location"); !strings.Contains(loc, "notice=") {
				t.Errorf("expected a notice explaining the rejection, got %q", loc)
			}
			if got := ts.deps.Settings.GetDefault(t.Context(), tc.key, ""); got != "http://192.168.0.2:8096" {
				t.Errorf("a rejected address overwrote the stored one: %q", got)
			}
		})
	}
}

// TestJellyfinTestButtonTellsTheTwoFailuresApart: before this, the button
// failed identically whether the address was wrong or the server simply had
// not been linked yet, and those are fixed in different places.
func TestJellyfinTestButtonTellsTheTwoFailuresApart(t *testing.T) {
	t.Parallel()

	t.Run("no address", func(t *testing.T) {
		t.Parallel()
		ts := newTestServer(t)
		resp := ts.get(t, "/settings/jellyfin/test", nil)
		if got := resp.Body; !strings.Contains(got, "Cargá primero la dirección") {
			t.Errorf("got %q", got)
		}
	})

	t.Run("address that answers nothing", func(t *testing.T) {
		t.Parallel()
		ts := newTestServer(t)
		// Port 1 on loopback: refuses immediately, so the test does not wait.
		if err := ts.deps.Settings.Set(t.Context(), settings.KeyJellyfinURL, "http://127.0.0.1:1"); err != nil {
			t.Fatal(err)
		}
		resp := ts.get(t, "/settings/jellyfin/test", nil)
		if got := resp.Body; !strings.Contains(got, "No se llega a esa dirección") {
			t.Errorf("got %q", got)
		}
	})
}

// TestQualityRefreshRejectsUnknownItems: the item id arrives from the browser
// and ends in a write on the media server, so it is checked against the report.
func TestQualityRefreshRejectsUnknownItems(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	// Linked, and an administrator, so the refusal cannot be about permissions.
	for key, value := range map[string]string{
		settings.KeyJellyfinURL:   "http://127.0.0.1:1",
		settings.KeyJellyfinToken: "tok",
		settings.KeyJellyfinAdmin: "true",
	} {
		if err := ts.deps.Settings.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}

	resp := ts.post(t, "/quality/refresh", url.Values{"item": {"../../System/Shutdown"}}, nil)
	if got := resp.Body; !strings.Contains(got, "Volvé a analizar") {
		t.Errorf("an id the report never saw should be refused, got %q", got)
	}
}

func TestQualityRefreshNeedsAnAdministrator(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	for key, value := range map[string]string{
		settings.KeyJellyfinURL:   "http://127.0.0.1:1",
		settings.KeyJellyfinToken: "tok",
		settings.KeyJellyfinAdmin: "false",
	} {
		if err := ts.deps.Settings.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}

	resp := ts.post(t, "/quality/refresh", url.Values{"item": {"whatever"}}, nil)
	if got := resp.Body; !strings.Contains(got, "Requiere admin") {
		t.Errorf("got %q", got)
	}
}
