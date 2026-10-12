package serverupdates

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// realFile is what ginebra-actualizar --estado wrote on Ginebra, trimmed, with
// one app in each state and a run that changed something.
const realFile = `{
 "generado": "2026-10-13T10:30:08+11:00",
 "cada_dias": 15,
 "ultima_actualizacion": "2026-10-13T10:30:01+11:00",
 "proxima": "2026-10-29T04:30:00+11:00",
 "corriendo": false,
 "reinicio_pendiente": true,
 "ultima_vuelta": {"fecha": "2026-10-13T10:30:01+11:00", "fallas": 1,
   "cambios": ["Seerr: v3.5.0 -> v3.5.1", "  ", "Bazarr: FALLO, se volvió a 1.6.2"]},
 "apps": [
  {"id": "debian", "nombre": "Debian", "instalada": "13.7", "ultima": "13.7", "estado": "nueva", "nota": "5 paquetes: openssl, libc6"},
  {"id": "jellyfin", "nombre": "Jellyfin", "instalada": "12.2", "ultima": "12.2", "estado": "al-dia", "nota": null},
  {"id": "radarr", "nombre": "Radarr", "instalada": "6.4.4.10685", "ultima": "7.0.0.1", "estado": "mayor", "nota": null},
  {"id": "trailarr", "nombre": "Trailarr", "instalada": "0.13.1", "ultima": null, "estado": "error", "nota": null}
 ]
}`

func TestParseTheServersFile(t *testing.T) {
	t.Parallel()
	st, err := Parse([]byte(realFile))
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 29, 4, 30, 0, 0, time.FixedZone("", 11*3600))
	if !st.Next.Equal(want) {
		t.Errorf("Next = %v, want %v", st.Next, want)
	}
	if st.Generated.IsZero() || st.LastUpdate.IsZero() {
		t.Errorf("dates not read: %+v", st)
	}
	if st.EveryDays != 15 || st.Running || !st.RebootPending {
		t.Errorf("flags: %+v", st)
	}
	if st.LastRun == nil || st.LastRun.Failures != 1 {
		t.Fatalf("last run: %+v", st.LastRun)
	}
	// The blank line is dropped, not shown as an empty change.
	if len(st.LastRun.Changes) != 2 || st.LastRun.Changes[0] != "Seerr: v3.5.0 -> v3.5.1" {
		t.Errorf("changes: %q", st.LastRun.Changes)
	}
	if len(st.Apps) != 4 {
		t.Fatalf("apps: %+v", st.Apps)
	}
	states := []State{StateNew, StateCurrent, StateMajor, StateError}
	for i, a := range st.Apps {
		if a.State != states[i] {
			t.Errorf("%s: state %q, want %q", a.Name, a.State, states[i])
		}
	}
	if st.Apps[0].Note != "5 paquetes: openssl, libc6" {
		t.Errorf("note: %q", st.Apps[0].Note)
	}
	if st.Apps[3].Latest != "" || st.Apps[3].Installed != "0.13.1" {
		t.Errorf("a null version should read as empty: %+v", st.Apps[3])
	}
}

// The script is a shell script with Python inside, on a machine that gets
// updated by it: the file has to be read leniently, and one bad field must not
// take the panel with it.
func TestParseIsLenient(t *testing.T) {
	t.Parallel()
	st, err := Parse([]byte(`{
		"generado": "ayer", "cada_dias": null, "proxima": null,
		"ultima_vuelta": null, "futuro": {"lo": "que sea"},
		"apps": [
			{"id": "x", "nombre": "X", "estado": "algo-nuevo"},
			{"id": "solo-id", "nombre": null, "estado": "al-dia"},
			{"id": "", "nombre": "", "estado": "al-dia"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !st.Generated.IsZero() || !st.Next.IsZero() || st.EveryDays != 0 {
		t.Errorf("malformed or null fields should read as zero: %+v", st)
	}
	if st.LastRun != nil {
		t.Errorf("a null run should stay nil: %+v", st.LastRun)
	}
	if len(st.Apps) != 2 {
		t.Fatalf("an app with neither id nor name should be skipped: %+v", st.Apps)
	}
	if st.Apps[0].State != StateError {
		t.Errorf("an unknown state is not one Holocron can vouch for: %q", st.Apps[0].State)
	}
	if st.Apps[1].Name != "solo-id" {
		t.Errorf("a missing name falls back to the id: %q", st.Apps[1].Name)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	t.Parallel()
	if _, err := Parse([]byte("<html>")); err == nil {
		t.Error("garbage should be an error, not an empty panel")
	}
}

func TestReadWithoutAFileIsNotExist(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"", filepath.Join(t.TempDir(), "nope.json")} {
		_, err := NewService(path, t.TempDir()).Read()
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("path %q: got %v, want os.ErrNotExist", path, err)
		}
	}
}

func TestReadTheFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "actualizaciones.json")
	if err := os.WriteFile(path, []byte(realFile), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewService(path, t.TempDir())
	if !s.Configured() {
		t.Fatal("a service with a path is configured")
	}
	st, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Apps) != 4 {
		t.Errorf("apps: %+v", st.Apps)
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	s := NewService("", t.TempDir())
	s.SetUnitDirForTest(t.TempDir())
	return s
}

func TestRequestCheckNeedsTheUnit(t *testing.T) {
	t.Parallel()
	s := newTestService(t)
	if s.CheckAvailable() {
		t.Fatal("no unit installed in this test")
	}
	if err := s.RequestCheck(); !errors.Is(err, ErrNoHelper) {
		t.Errorf("got %v, want ErrNoHelper", err)
	}
	entries, err := os.ReadDir(s.StateDirForTest())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("a refused request left a trigger behind: %v", entries)
	}
}

func TestRequestCheckDropsAnEmptyTrigger(t *testing.T) {
	t.Parallel()
	s := newTestService(t)
	if err := os.WriteFile(filepath.Join(s.unitDir, unitName), []byte("[Path]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !s.CheckAvailable() {
		t.Fatal("the unit is installed")
	}
	if _, ok := s.CheckPending(); ok {
		t.Fatal("nothing requested yet")
	}
	if err := s.RequestCheck(); err != nil {
		t.Fatal(err)
	}
	// The same name the server's path unit watches, and empty: a signal, not
	// a message.
	info, err := os.Stat(filepath.Join(s.StateDirForTest(), ".check-updates-requested"))
	if err != nil {
		t.Fatalf("trigger not written: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("the trigger should be empty, has %d bytes", info.Size())
	}
	if _, ok := s.CheckPending(); !ok {
		t.Error("a fresh request should read as pending")
	}
}

func TestAStaleTriggerIsNotPending(t *testing.T) {
	t.Parallel()
	s := newTestService(t)
	path := filepath.Join(s.StateDirForTest(), triggerName)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.CheckPending(); ok {
		t.Error("a trigger nobody picked up in ten minutes is not in flight")
	}
}
