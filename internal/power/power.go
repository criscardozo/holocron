// Package power asks a privileged helper to act on the machine Holocron runs
// on: shut it down, restart it, or restart one of the services beside it.
//
// The service cannot do any of this itself, and that is deliberate. Its unit
// carries NoNewPrivileges=true and ProtectSystem=strict, which is also why
// sudo is not an option: the kernel ignores the setuid bit under that flag, so
// sudo cannot work from this process even if the user were in sudoers. The
// arrangement is the one the updater already uses — Holocron touches a file, a
// root path unit notices and acts.
//
// # Where the authority sits
//
// Each action has its own trigger file and its own pair of systemd units, and
// the trigger is empty. It is a signal, not a message: the root unit's
// ExecStart is fixed at install time and never reads what Holocron wrote.
//
// That distinction is the whole point. Naming the action *inside* one shared
// trigger would put a root process in the business of parsing data written by
// a web server, and the safety of the thing would rest on an allowlist living
// in this package — the exact arrangement the pattern exists to avoid. With one
// unit per action, the set of possible actions is the set of installed units,
// auditable from outside Holocron with `systemctl list-units 'holocron-*'`, and
// there is not a line of parsing anywhere.
package power

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Action is one thing the machine can be asked to do. Each maps to a trigger
// file and a pair of units installed by scripts/install.sh.
type Action string

// The actions, in the order the screen shows them. Restarts come first because
// they are the ones worth reaching for often; the two that take the machine
// away are last.
const (
	ActionRestartJellyfin   Action = "restart-jellyfin"
	ActionRestartQbit       Action = "restart-qbittorrent"
	ActionRestartCloudflare Action = "restart-cloudflared"
	ActionRestartHolocron   Action = "restart-holocron"
	ActionReboot            Action = "reboot"
	ActionPowerOff          Action = "poweroff"
)

// Actions in display order.
var Actions = []Action{
	ActionRestartJellyfin,
	ActionRestartQbit,
	ActionRestartCloudflare,
	ActionRestartHolocron,
	ActionReboot,
	ActionPowerOff,
}

// Label is the button text.
func (a Action) Label() string {
	switch a {
	case ActionRestartJellyfin:
		return "Reiniciar Jellyfin"
	case ActionRestartQbit:
		return "Reiniciar qBittorrent"
	case ActionRestartCloudflare:
		return "Reiniciar el túnel"
	case ActionRestartHolocron:
		return "Reiniciar Holocron"
	case ActionReboot:
		return "Reiniciar la Pi"
	case ActionPowerOff:
		return "Apagar la Pi"
	default:
		return string(a)
	}
}

// Detail says what the action costs. This is the only warning before something
// disruptive, so it names the consequence rather than asking "are you sure".
func (a Action) Detail() string {
	switch a {
	case ActionRestartJellyfin:
		return "Corta lo que se esté reproduciendo. Vuelve solo en unos segundos."
	case ActionRestartQbit:
		return "Las descargas se reanudan solas al volver."
	case ActionRestartCloudflare:
		return "Corta el acceso por el dominio público — incluido este, si entraste por ahí. Desde la LAN no se nota."
	case ActionRestartHolocron:
		return "Esta página deja de responder unos segundos. Recargá."
	case ActionReboot:
		return "Se va todo por un minuto o dos. Vuelve sola."
	case ActionPowerOff:
		// The sentence that matters on this screen: a Pi 4 has no
		// wake-on-LAN, so this is one-way unless someone is there.
		return "No se puede encender a distancia: queda apagada hasta que alguien vaya hasta ella. Si estás fuera de casa, te quedás sin Jellyfin hasta volver."
	default:
		return ""
	}
}

// Confirm is the question the browser asks first.
func (a Action) Confirm() string {
	return "¿" + a.Label() + "? " + a.Detail()
}

// NeedsToken marks the one action that also asks for the API token. The line is
// irreversibility, not disruption: everything else here comes back on its own.
//
// Deliberately only one. Giving reboot the same friction would train the same
// gesture for both, and the one that bites is the one that does not recover.
// Typing a token breaks the automatism in a way a second tap does not.
func (a Action) NeedsToken() bool {
	return a == ActionPowerOff
}

// Interrupts reports whether the whole machine goes away, rather than one
// service.
func (a Action) Interrupts() bool {
	return a == ActionReboot || a == ActionPowerOff
}

// Icon is the sprite symbol for the action.
func (a Action) Icon() string {
	switch a {
	case ActionPowerOff:
		return "power"
	case ActionReboot:
		return "refresh"
	default:
		return "plug"
	}
}

// trigger is the file this action signals through. Empty by design; see the
// package comment.
func (a Action) trigger() string { return "." + string(a) + "-requested" }

// unit is the path unit that watches for it. Its presence is what makes the
// action offerable at all.
func (a Action) unit() string {
	return "/etc/systemd/system/holocron-" + string(a) + ".path"
}

// Valid reports whether s names an action.
func Valid(s string) (Action, bool) {
	for _, a := range Actions {
		if string(a) == s {
			return a, true
		}
	}
	return "", false
}

// ErrNoHelper means the units for this action are not installed. Reported
// plainly rather than as a failure: the fix is to re-run the installer.
var ErrNoHelper = errors.New("the privileged helper for this action is not installed")

// pendingFor is how long a request counts as still in flight. Each unit removes
// its own trigger before doing anything, so a file older than this means the
// path unit never fired.
const pendingFor = 90 * time.Second

// Service requests machine-level actions.
type Service struct {
	stateDir string
	// unitDir is where the path units live. A field so tests can point it
	// somewhere writable.
	unitDir string
}

// NewService creates a Service. stateDir is the one directory the hardened unit
// can write to, which is where the triggers go.
func NewService(stateDir string) *Service {
	return &Service{stateDir: stateDir, unitDir: "/etc/systemd/system"}
}

// Available reports whether this particular action has its units installed. Per
// action rather than all-or-nothing, so an older install that only has some of
// them shows what it can actually do.
func (s *Service) Available(a Action) bool {
	name := filepath.Base(a.unit())
	_, err := os.Stat(filepath.Join(s.unitDir, name))
	return err == nil
}

// Installed reports whether any action at all is available.
func (s *Service) Installed() bool {
	for _, a := range Actions {
		if s.Available(a) {
			return true
		}
	}
	return false
}

// Request asks for an action by touching its trigger file. The file is empty:
// nothing reads its contents, and nothing should.
func (s *Service) Request(a Action) error {
	if _, ok := Valid(string(a)); !ok {
		return fmt.Errorf("unknown action %q", a)
	}
	if !s.Available(a) {
		return ErrNoHelper
	}
	path := filepath.Join(s.stateDir, a.trigger())
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		return fmt.Errorf("request %s: %w", a, err)
	}
	return nil
}

// Pending reports an action already on its way, so the screen says so instead
// of inviting a second press into the void.
func (s *Service) Pending() (Action, bool) {
	for _, a := range Actions {
		info, err := os.Stat(filepath.Join(s.stateDir, a.trigger()))
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > pendingFor {
			// Stale: each unit clears its trigger first, so a file this old
			// means it never ran. Claiming "in progress" forever would leave
			// the screen stuck with no way to try again.
			continue
		}
		return a, true
	}
	return "", false
}

// SetUnitDirForTest points the service at a different unit directory. Only for
// tests: production always reads /etc/systemd/system.
func (s *Service) SetUnitDirForTest(dir string) { s.unitDir = dir }

// StateDirForTest exposes where the triggers are written, so a test can assert
// that a refused action left nothing behind.
func (s *Service) StateDirForTest() string { return s.stateDir }
