// Package serverupdates reads what the server says about its own updates and
// asks it to look again.
//
// Holocron does not check versions itself. A root script on the server
// (ginebra-actualizar --estado on Ginebra) knows how each app is installed and
// where its releases come from, and writes the result to a JSON file Holocron's
// group can read, the same arrangement as the SMART snapshot. This package only
// reads that file.
//
// Looking again follows the convention internal/power uses for its actions: an
// empty trigger file in Holocron's state directory, picked up by a root path
// unit whose presence is what makes the button offerable. It lives here rather
// than among the power actions because it is nothing like them: it interrupts
// nothing, asks for no confirmation and no token, and has no business on the
// list of things that take the machine or a service away — nor in the iOS
// app's copy of that list.
package serverupdates

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// State is what the server concluded about one app.
type State string

// The states the server writes. Anything else is read as StateError: a state
// this version of Holocron does not know is not one it can vouch for.
const (
	// StateCurrent: the installed version is the latest.
	StateCurrent State = "al-dia"
	// StateNew: a newer version exists and the next automatic run installs it.
	StateNew State = "nueva"
	// StateMajor: a new major version exists. The automatic run never crosses
	// one; it has to be done by hand.
	StateMajor State = "mayor"
	// StateError: the server could not find out.
	StateError State = "error"
)

// App is one piece of the server: the system, or one of the apps on it.
type App struct {
	ID        string
	Name      string
	Installed string
	Latest    string
	State     State
	// Note is free text from the server, such as which Debian packages are
	// waiting.
	Note string
}

// Run is what the last update run did.
type Run struct {
	At       time.Time
	Failures int
	// Changes are lines such as "Seerr: v3.5.0 -> v3.5.1"; a failed one says
	// FALLO.
	Changes []string
}

// Status is the whole file.
type Status struct {
	// Generated is when the server last looked. Zero when the file does not
	// say.
	Generated time.Time
	// EveryDays is how often the automatic update runs.
	EveryDays  int
	LastUpdate time.Time
	// Next is when the automatic update runs again.
	Next time.Time
	// Running is true while an update is being installed.
	Running bool
	// RebootPending is true when a new kernel is installed but not booted.
	RebootPending bool
	// LastRun is nil when the server has no record of one.
	LastRun *Run
	Apps    []App
}

// file mirrors the JSON. Times are strings and parsed one by one, so a single
// malformed or null date costs that date and not the whole panel; every other
// field tolerates null because decoding null into a Go value leaves it alone.
type file struct {
	Generated     string    `json:"generado"`
	EveryDays     int       `json:"cada_dias"`
	LastUpdate    string    `json:"ultima_actualizacion"`
	Next          string    `json:"proxima"`
	Running       bool      `json:"corriendo"`
	RebootPending bool      `json:"reinicio_pendiente"`
	LastRun       *fileRun  `json:"ultima_vuelta"`
	Apps          []fileApp `json:"apps"`
}

type fileRun struct {
	At       string   `json:"fecha"`
	Failures int      `json:"fallas"`
	Changes  []string `json:"cambios"`
}

type fileApp struct {
	ID        string `json:"id"`
	Name      string `json:"nombre"`
	Installed string `json:"instalada"`
	Latest    string `json:"ultima"`
	State     string `json:"estado"`
	Note      string `json:"nota"`
}

// Parse reads the server's JSON.
func Parse(b []byte) (*Status, error) {
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("decode updates status: %w", err)
	}
	st := &Status{
		Generated:     parseTime(f.Generated),
		EveryDays:     f.EveryDays,
		LastUpdate:    parseTime(f.LastUpdate),
		Next:          parseTime(f.Next),
		Running:       f.Running,
		RebootPending: f.RebootPending,
	}
	if f.LastRun != nil {
		run := &Run{At: parseTime(f.LastRun.At), Failures: f.LastRun.Failures}
		for _, c := range f.LastRun.Changes {
			if c = strings.TrimSpace(c); c != "" {
				run.Changes = append(run.Changes, c)
			}
		}
		st.LastRun = run
	}
	for _, a := range f.Apps {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			name = strings.TrimSpace(a.ID)
		}
		if name == "" {
			continue
		}
		st.Apps = append(st.Apps, App{
			ID:        strings.TrimSpace(a.ID),
			Name:      name,
			Installed: strings.TrimSpace(a.Installed),
			Latest:    strings.TrimSpace(a.Latest),
			State:     parseState(a.State),
			Note:      strings.TrimSpace(a.Note),
		})
	}
	return st, nil
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseState(s string) State {
	switch st := State(strings.TrimSpace(s)); st {
	case StateCurrent, StateNew, StateMajor, StateError:
		return st
	default:
		return StateError
	}
}

const (
	// triggerName is the file holocron-check-updates.path watches for. Empty
	// by design, like every trigger: see internal/power.
	triggerName = ".check-updates-requested"
	// unitName is the path unit. Installed by the server, not by Holocron's
	// installer, because what it runs is the server's own script.
	unitName = "holocron-check-updates.path"
	// pendingFor is how long a request counts as still in flight. The unit
	// removes the trigger before it starts looking, so one older than this
	// means it never fired.
	pendingFor = 90 * time.Second
)

// ErrNoHelper means the path unit is not installed, so nobody would pick the
// request up.
var ErrNoHelper = errors.New("the check-updates helper is not installed")

// Service reads the status file and asks for a new check.
type Service struct {
	path     string
	stateDir string
	unitDir  string
}

// NewService creates a Service. path is HOLOCRON_UPDATES_FILE (empty turns the
// feature off); stateDir is where the trigger goes, the one directory the
// hardened unit can write to.
func NewService(path, stateDir string) *Service {
	return &Service{path: path, stateDir: stateDir, unitDir: "/etc/systemd/system"}
}

// Configured reports whether the server said where the file is.
func (s *Service) Configured() bool { return s.path != "" }

// Read reads the status file. An unconfigured service or a missing file is
// os.ErrNotExist, which callers treat as "nothing to show" rather than as a
// failure.
func (s *Service) Read() (*Status, error) {
	if s.path == "" {
		return nil, os.ErrNotExist
	}
	b, err := os.ReadFile(s.path) //#nosec G304 -- path from server configuration
	if err != nil {
		return nil, fmt.Errorf("read updates status: %w", err)
	}
	return Parse(b)
}

// CheckAvailable reports whether the path unit that answers a request is
// installed.
func (s *Service) CheckAvailable() bool {
	_, err := os.Stat(filepath.Join(s.unitDir, unitName))
	return err == nil
}

// RequestCheck asks the server to look again by touching the trigger file.
func (s *Service) RequestCheck() error {
	if !s.CheckAvailable() {
		return ErrNoHelper
	}
	if err := os.WriteFile(filepath.Join(s.stateDir, triggerName), nil, 0o600); err != nil {
		return fmt.Errorf("request update check: %w", err)
	}
	return nil
}

// CheckPending reports a request not yet picked up, and when it was made, so a
// page loaded in the meantime says it is on its way instead of offering the
// button again.
func (s *Service) CheckPending() (time.Time, bool) {
	info, err := os.Stat(filepath.Join(s.stateDir, triggerName))
	if err != nil {
		return time.Time{}, false
	}
	if time.Since(info.ModTime()) > pendingFor {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

// SetUnitDirForTest points the service at a different unit directory. Only for
// tests: production always reads /etc/systemd/system.
func (s *Service) SetUnitDirForTest(dir string) { s.unitDir = dir }

// StateDirForTest exposes where the trigger is written.
func (s *Service) StateDirForTest() string { return s.stateDir }
