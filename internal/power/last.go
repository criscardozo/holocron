package power

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// On Ginebra the power-off and reboot units do not call systemctl directly:
// they run the server's own scripts, which go through a preflight that refuses
// while somebody is watching, a torrent is active or Jellyfin is running a
// task. When they refuse, the machine simply does not go down — and from
// Holocron's side that looks exactly like the request getting lost.
//
// So the scripts leave their answer in this file, and the management screen
// shows it. Written by root, 0644, in Holocron's own state directory.
const lastActionFile = ".last-action.json"

// LastAction is what the server's power script last decided.
type LastAction struct {
	Action string    `json:"action"` // poweroff | reboot
	OK     bool      `json:"ok"`
	Reason string    `json:"reason"` // the preflight's reasons, joined with "; "
	At     time.Time `json:"at"`
}

// Last reads the server's answer to the last power request. ok is false when
// there is none, which is normal on a machine without the server scripts.
func (s *Service) Last() (LastAction, bool) {
	b, err := os.ReadFile(filepath.Join(s.stateDir, lastActionFile))
	if err != nil {
		return LastAction{}, false
	}
	var a LastAction
	if err := json.Unmarshal(b, &a); err != nil || a.At.IsZero() {
		return LastAction{}, false
	}
	return a, true
}
