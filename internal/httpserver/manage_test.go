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

// asHost sends the request with a Host header of our choosing. It cannot go
// through the headers map: net/http reads Host off the request field and
// ignores a header by that name, so a test that set it there would pass while
// exercising the LAN path.
func asHost(t *testing.T, ts *testServer, host, path string, form url.Values) response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		ts.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = host
	return ts.do(t, req)
}

func getAsHost(t *testing.T, ts *testServer, host, path string) response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	return ts.do(t, req)
}

// tokenFor generates the API token and returns it, for the tests that need to
// get past the token gate to reach what they are actually testing.
func tokenFor(t *testing.T, ts *testServer) string {
	t.Helper()
	tok, err := ts.deps.APIToken.Generate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// TestPoweringOffFromOutsideAsksYouToSayIt covers the case that used to be
// refused outright. It is allowed now, but not silently: the machine cannot be
// woken over the network, so the consequence has to be stated.
func TestPoweringOffFromOutsideAsksYouToSayIt(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	installHelper(t, ts)
	token := tokenFor(t, ts)

	const public = "holocron.merli.store"

	resp := asHost(t, ts, public, "/manage/action",
		url.Values{"action": {"poweroff"}, "token": {token}})
	if !strings.Contains(resp.Body, "dirección pública") {
		t.Errorf("expected the public-address warning, got %q", resp.Body)
	}
	if requested(t, ts, power.ActionPowerOff) {
		t.Fatal("powered off from outside without the acknowledgement")
	}

	resp = asHost(t, ts, public, "/manage/action",
		url.Values{"action": {"poweroff"}, "token": {token}, "ack": {"1"}})
	if !requested(t, ts, power.ActionPowerOff) {
		t.Fatalf("the acknowledged power-off was not requested: %q", resp.Body)
	}
}

// TestBeingHomeDoesNotAskForTheAck guards the other direction: the new gate
// must not make the ordinary case — standing next to the machine — worse.
func TestBeingHomeDoesNotAskForTheAck(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	installHelper(t, ts)
	token := tokenFor(t, ts)

	resp := asHost(t, ts, "192.168.0.2:8080", "/manage/action",
		url.Values{"action": {"poweroff"}, "token": {token}})
	if !requested(t, ts, power.ActionPowerOff) {
		t.Fatalf("a power-off from the LAN should not need an ack: %q", resp.Body)
	}
}

// TestOnlyStrandingAsksForTheAck keeps the friction attached to the property
// that earns it. Rebooting from a train is fine — it comes back on its own.
func TestOnlyStrandingAsksForTheAck(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	installHelper(t, ts)

	resp := asHost(t, ts, "holocron.merli.store", "/manage/action",
		url.Values{"action": {"reboot"}})
	if !requested(t, ts, power.ActionReboot) {
		t.Fatalf("a remote reboot should not need an ack: %q", resp.Body)
	}
}

// TestTheAckOnlyAppearsFromOutside checks the rendering matches the rule, so
// the LAN form does not grow a checkbox nobody needs to tick.
func TestTheAckOnlyAppearsFromOutside(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	installHelper(t, ts)

	if body := getAsHost(t, ts, "192.168.0.2:8080", "/manage").Body; strings.Contains(body, `name="ack"`) {
		t.Error("the LAN page should not ask for an acknowledgement")
	}
	body := getAsHost(t, ts, "holocron.merli.store", "/manage").Body
	if !strings.Contains(body, `name="ack"`) {
		t.Error("the public page should ask for an acknowledgement")
	}
	if !strings.Contains(body, "required") {
		t.Error("the acknowledgement must be required, so the browser enforces it too")
	}
}
