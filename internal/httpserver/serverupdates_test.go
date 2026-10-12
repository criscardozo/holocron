package httpserver

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// updatesJSON is a status file generated at gen, with one app in each state.
func updatesJSON(gen time.Time, running bool) string {
	return fmt.Sprintf(`{
 "generado": %q, "cada_dias": 15,
 "ultima_actualizacion": %q, "proxima": %q,
 "corriendo": %t, "reinicio_pendiente": true,
 "ultima_vuelta": {"fecha": %q, "fallas": 1, "cambios": ["Seerr: v3.5.0 -> v3.5.1", "Bazarr: FALLO"]},
 "apps": [
  {"id": "debian", "nombre": "Debian", "instalada": "13.7", "ultima": "13.7", "estado": "nueva", "nota": "5 paquetes: openssl"},
  {"id": "jellyfin", "nombre": "Jellyfin", "instalada": "12.2", "ultima": "12.2", "estado": "al-dia", "nota": null},
  {"id": "radarr", "nombre": "Radarr", "instalada": "6.4.4", "ultima": "7.0.0", "estado": "mayor", "nota": null},
  {"id": "trailarr", "nombre": "Trailarr", "instalada": "0.13.1", "ultima": null, "estado": "error", "nota": null}
 ]
}`, gen.Format(time.RFC3339), gen.Add(-time.Hour).Format(time.RFC3339),
		gen.Add(15*24*time.Hour).Format(time.RFC3339), running, gen.Add(-time.Hour).Format(time.RFC3339))
}

func writeUpdates(t *testing.T, ts *testServer, gen time.Time, running bool) {
	t.Helper()
	if err := os.WriteFile(ts.updatesFile, []byte(updatesJSON(gen, running)), 0o600); err != nil {
		t.Fatal(err)
	}
}

// installCheckUnit fakes the server's holocron-check-updates.path.
func installCheckUnit(t *testing.T, ts *testServer) {
	t.Helper()
	units := t.TempDir()
	if err := os.WriteFile(filepath.Join(units, "holocron-check-updates.path"), []byte("[Path]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ts.deps.ServerUpdates.SetUnitDirForTest(units)
}

func checkTriggered(t *testing.T, ts *testServer) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(ts.deps.ServerUpdates.StateDirForTest(), ".check-updates-requested"))
	return err == nil
}

func TestUpdatesPanelHiddenWithoutTheFile(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	resp := ts.get(t, "/manage", nil)
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d", resp.Status)
	}
	if strings.Contains(resp.Body, `id="server-updates"`) {
		t.Error("no file means no panel, not an empty one")
	}
}

func TestUpdatesPanelShowsEachState(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	writeUpdates(t, ts, time.Now().Add(-5*time.Minute), false)

	body := ts.get(t, "/manage", nil).Body
	for _, want := range []string{
		`id="server-updates"`,
		"versión nueva", "versión mayor: a mano", "no se pudo revisar", "al día",
		"5 paquetes: openssl", // the note
		"7.0.0",               // the latest version of the one behind
		"Revisado hace 5 min",
		"Próxima:", "días (", // the date of the next run, whatever it is
		"kernel nuevo", // reboot pending
		"Última vuelta", "2 cambios", "1 falla", "upd-fail",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("panel lacks %q", want)
		}
	}
	// Not installed on this test machine, so no button that leads nowhere.
	if strings.Contains(body, "Buscar actualizaciones") {
		t.Error("the button should hide without the server's path unit")
	}
	// Power actions are not where this lives.
	if strings.Contains(body, `value="check-updates"`) {
		t.Error("checking for updates must not appear among the power actions")
	}
}

func TestCheckForUpdatesDropsTheTriggerAndPolls(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	installCheckUnit(t, ts)
	gen := time.Now().Add(-time.Hour).Truncate(time.Second)
	writeUpdates(t, ts, gen, false)

	if body := ts.get(t, "/manage", nil).Body; !strings.Contains(body, "Buscar actualizaciones") {
		t.Fatal("the button should show once the unit is installed")
	}

	resp := ts.post(t, "/manage/updates/check", url.Values{}, nil)
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d", resp.Status)
	}
	if !checkTriggered(t, ts) {
		t.Fatal("the request should leave the trigger the path unit watches")
	}
	if !strings.Contains(resp.Body, "Revisando") || !strings.Contains(resp.Body, `hx-trigger="every 2s"`) {
		t.Errorf("expected a polling «Revisando…», got %q", resp.Body)
	}
	if strings.Contains(resp.Body, "Buscar actualizaciones") {
		t.Error("no second button while the first request is under way")
	}
	poll := fmt.Sprintf("/manage/updates?since=%d&amp;started=", gen.Unix())
	if !strings.Contains(resp.Body, poll) {
		t.Fatalf("the poll should carry the file's date, want %q in %q", poll, resp.Body)
	}

	// Still the old file: keep polling.
	started := time.Now().Unix()
	still := ts.get(t, fmt.Sprintf("/manage/updates?since=%d&started=%d", gen.Unix(), started), nil)
	if !strings.Contains(still.Body, "Revisando") {
		t.Errorf("an unchanged file means still checking, got %q", still.Body)
	}

	// The server rewrote it: stop, and offer the button again.
	writeUpdates(t, ts, time.Now(), false)
	done := ts.get(t, fmt.Sprintf("/manage/updates?since=%d&started=%d", gen.Unix(), started), nil)
	if strings.Contains(done.Body, "Revisando") || strings.Contains(done.Body, "every 2s") {
		t.Errorf("a rewritten file ends the polling, got %q", done.Body)
	}
	if !strings.Contains(done.Body, "Revisado hace instantes") || !strings.Contains(done.Body, "Buscar actualizaciones") {
		t.Errorf("expected the fresh panel, got %q", done.Body)
	}
}

func TestCheckThatNeverAnswersStopsPolling(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	installCheckUnit(t, ts)
	gen := time.Now().Add(-time.Hour)
	writeUpdates(t, ts, gen, false)

	started := time.Now().Add(-4 * time.Minute).Unix()
	body := ts.get(t, fmt.Sprintf("/manage/updates?since=%d&started=%d", gen.Unix(), started), nil).Body
	if strings.Contains(body, "every 2s") {
		t.Error("polling should give up after the timeout")
	}
	if !strings.Contains(body, "tardando más de lo normal") {
		t.Errorf("expected the gentle timeout message, got %q", body)
	}
}

func TestCheckWithoutTheUnitIsRefused(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	writeUpdates(t, ts, time.Now(), false)

	resp := ts.post(t, "/manage/updates/check", url.Values{}, nil)
	if !strings.Contains(resp.Body, "holocron-check-updates.path") {
		t.Errorf("expected the missing unit to be named, got %q", resp.Body)
	}
	if checkTriggered(t, ts) {
		t.Error("a refused request left a trigger behind")
	}
}

func TestCheckFromAnotherSiteIsRefused(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	installCheckUnit(t, ts)
	writeUpdates(t, ts, time.Now(), false)

	resp := ts.post(t, "/manage/updates/check", url.Values{},
		map[string]string{"Sec-Fetch-Site": "cross-site"})
	if resp.Status == http.StatusOK {
		t.Errorf("a cross-site POST should be refused, got %d", resp.Status)
	}
	if checkTriggered(t, ts) {
		t.Error("a cross-site request reached the server")
	}
}

func TestInstallingPollsUntilItEnds(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	installCheckUnit(t, ts)
	writeUpdates(t, ts, time.Now(), true)

	body := ts.get(t, "/manage/updates", nil).Body
	if !strings.Contains(body, "Instalando actualizaciones") || !strings.Contains(body, `hx-get="/manage/updates"`) {
		t.Errorf("expected a polling «Instalando…», got %q", body)
	}
	if strings.Contains(body, "Buscar actualizaciones") {
		t.Error("the server will not look while it installs; no button meanwhile")
	}

	writeUpdates(t, ts, time.Now(), false)
	if body := ts.get(t, "/manage/updates", nil).Body; strings.Contains(body, "every 2s") {
		t.Errorf("polling should stop when the install ends, got %q", body)
	}
}

func TestWhenPhrases(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 13, 10, 0, 0, 0, time.Local)
	for _, c := range []struct {
		got, want string
	}{
		{whenPast(now.Add(-30*time.Minute), now), "hoy a las 09:30"},
		{whenPast(now.Add(-20*time.Hour), now), "ayer a las 14:00"},
		{whenPast(time.Date(2026, 10, 10, 4, 30, 0, 0, time.Local), now), "hace 3 días (10 de octubre)"},
		{whenNext(now.Add(time.Hour), now), "hoy a las 11:00"},
		// Tomorrow at 04:30 is less than a day away and still tomorrow.
		{whenNext(time.Date(2026, 10, 14, 4, 30, 0, 0, time.Local), now), "mañana a las 04:30"},
		{whenNext(time.Date(2026, 10, 29, 4, 30, 0, 0, time.Local), now), "en 16 días (29 de octubre)"},
		{whenNext(now.Add(-time.Hour), now), "pendiente (tocaba el 13 de octubre)"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}
