package templates

// ServerUpdatesView drives the Actualizaciones panel on the Gestión page: what
// the server says about its own updates, and the button that asks it to look
// again.
type ServerUpdatesView struct {
	// Show is false when the server does not publish the file, which hides
	// the panel rather than showing an empty one.
	Show bool
	// Error is set when the file exists but could not be read.
	Error string
	// Notice answers a button press, such as a request the server could not
	// take.
	Notice    string
	NoticeErr bool
	Machine   string

	// The summary line. Each is empty when the file does not say.
	LastUpdate string
	Next       string
	Checked    string

	// How many apps are not current, by kind, for the badges in the heading.
	New, Major, Failed int

	// Running is true while an update is being installed; RebootPending when
	// a new kernel waits for a reboot.
	Running       bool
	RebootPending bool

	Apps    []ServerUpdateRow
	LastRun *ServerUpdateRun

	// CanCheck shows the button: the server's path unit is installed and
	// nothing is already under way.
	CanCheck bool
	// Checking is true between the button press and the server rewriting the
	// file. CheckSlow replaces it once that has taken longer than it should.
	Checking  bool
	CheckSlow bool
	// PollURL is what the panel re-fetches every 2 s while something is under
	// way; empty when nothing is.
	PollURL string
}

// AllCurrent reports whether there is nothing to say about any app.
func (v ServerUpdatesView) AllCurrent() bool {
	return len(v.Apps) > 0 && v.New == 0 && v.Major == 0 && v.Failed == 0
}

// ServerUpdateRow is one app.
type ServerUpdateRow struct {
	Name      string
	Installed string
	// Latest is empty when it is the same as Installed or unknown, so the row
	// shows one version instead of "12.2 → 12.2".
	Latest string
	Note   string
	Badge  string
	// BadgeClass is one of the badge colours; Attention highlights the row.
	BadgeClass string
	Attention  bool
}

// ServerUpdateRun is what the last update run changed.
type ServerUpdateRun struct {
	// Title is the folded line: when it ran and how many changes.
	Title    string
	Failures int
	Changes  []ServerUpdateChange
}

// ServerUpdateChange is one line of it.
type ServerUpdateChange struct {
	Text   string
	Failed bool
}
