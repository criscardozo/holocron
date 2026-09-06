package httpserver

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cristian/holocron/internal/power"
)

// installHelper fakes the privileged units being present, which is all the
// service checks before offering an action.
func installHelper(t *testing.T, ts *testServer) string {
	t.Helper()
	units := t.TempDir()
	for _, a := range power.Actions {
		name := "holocron-" + string(a) + ".path"
		if err := os.WriteFile(filepath.Join(units, name), []byte("[Path]\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ts.deps.Power.SetUnitDirForTest(units)
	return units
}

// TestPoweringOffNeedsTheToken is the one that matters. On the LAN the web has
// no authentication at all, so without this anybody on the network could take
// the machine away — and a Pi 4 cannot be woken remotely, so getting it back
// means walking to it.
func TestPoweringOffNeedsTheToken(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	installHelper(t, ts)

	if _, err := ts.deps.APIToken.Generate(t.Context()); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ name, token, want string }{
		{"no token", "", "Pegá el token"},
		{"wrong token", "nope", "no coincide"},
	} {
		resp := ts.post(t, "/manage/action",
			url.Values{"action": {"poweroff"}, "token": {tc.token}}, nil)
		if !strings.Contains(resp.Body, tc.want) {
			t.Errorf("%s: expected %q, got %q", tc.name, tc.want, resp.Body)
		}
		if requested(t, ts, power.ActionPowerOff) {
			t.Fatalf("%s: the machine was asked to power off anyway", tc.name)
		}
	}

	if !strings.Contains(ts.logs.String(), "rejected machine action") {
		t.Error("a refused power-off must be logged")
	}
}

// TestRebootDoesNotShareTheTokenGate: identical friction would train one
// gesture for both, and reboot is the recoverable one.
func TestRebootDoesNotShareTheTokenGate(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	installHelper(t, ts)

	resp := ts.post(t, "/manage/action", url.Values{"action": {"reboot"}}, nil)
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d", resp.Status)
	}
	if !requested(t, ts, power.ActionReboot) {
		t.Error("reboot should go through on the confirmation alone")
	}
	if !strings.Contains(resp.Body, "vuelve sola") {
		t.Errorf("the acknowledgement should say what to expect, got %q", resp.Body)
	}
}

func TestValidTokenPowersOff(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	installHelper(t, ts)

	token, err := ts.deps.APIToken.Generate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	resp := ts.post(t, "/manage/action",
		url.Values{"action": {"poweroff"}, "token": {token}}, nil)

	if !requested(t, ts, power.ActionPowerOff) {
		t.Fatal("a valid token should reach the helper")
	}
	// The request that stops the machine cannot report how it went, so the
	// page has to explain how the user will know.
	if !strings.Contains(resp.Body, "deje de responder") {
		t.Errorf("expected the page to say how to tell it worked, got %q", resp.Body)
	}
}

func TestUnknownActionIsRefused(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	installHelper(t, ts)

	for _, bogus := range []string{"", "shutdown", "rm -rf /", "POWEROFF"} {
		resp := ts.post(t, "/manage/action", url.Values{"action": {bogus}}, nil)
		if !strings.Contains(resp.Body, "no existe") {
			t.Errorf("action %q: got %q", bogus, resp.Body)
		}
	}
	entries, err := os.ReadDir(ts.deps.Power.StateDirForTest())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("a refused action left triggers behind: %v", entries)
	}
}

// TestManagePageWithoutTheHelper: the whole feature is missing rather than one
// action failing, and the page says so instead of showing dead buttons.
func TestManagePageWithoutTheHelper(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)

	page := ts.get(t, "/manage", nil).Body
	if !strings.Contains(page, "no está instalado") {
		t.Errorf("expected the page to explain the helper is missing, got %q", page)
	}
	if strings.Contains(page, "Apagar la Pi") {
		t.Error("no buttons should be offered when nothing can act on them")
	}
}

// TestPowerOffWarningNamesTheRealCost: a Pi 4 has no wake-on-LAN. Pressing this
// from outside the house means no Jellyfin until someone gets home, and the
// page has to say that rather than "are you sure".
func TestPowerOffWarningNamesTheRealCost(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	installHelper(t, ts)

	page := ts.get(t, "/manage", nil).Body
	for _, phrase := range []string{"no se puede encender a distancia", "fuera de casa"} {
		if !strings.Contains(strings.ToLower(page), phrase) {
			t.Errorf("the page should warn about %q", phrase)
		}
	}
}

// TestPreflightDoesNotClaimAllClearWhenItCouldNotAsk: with nothing configured,
// there is no way to know whether someone is watching something, and saying
// "nada en curso" would be a claim the server cannot back.
func TestPreflightDoesNotClaimAllClearWhenItCouldNotAsk(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	installHelper(t, ts)

	page := ts.get(t, "/manage", nil).Body
	if strings.Contains(page, "Nada en curso") {
		t.Error("nothing could be consulted, so this is not an all-clear")
	}
	if !strings.Contains(page, "No se pudo consultar") {
		t.Errorf("expected the page to admit it does not know, got %q", page)
	}
}

func requested(t *testing.T, ts *testServer, a power.Action) bool {
	t.Helper()
	pending, ok := ts.deps.Power.Pending()
	return ok && pending == a
}
