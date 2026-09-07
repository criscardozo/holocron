package templates

// ManageActionRow is one thing that can be done to the machine.
type ManageActionRow struct {
	Key    string
	Label  string
	Detail string
	// Confirm is the question the browser asks first. Every action here is
	// disruptive; none of them should happen on a stray tap.
	Confirm string
	Icon    string
	// Destructive marks the ones that also require the API token, which is
	// what makes the form ask for it.
	Destructive bool
	// RequireAck asks the person to state the consequence out loud before the
	// button works. Set only when the action strands the machine *and* the
	// page was reached from outside the house — the one combination where the
	// consequence is not obvious from where you are standing.
	RequireAck bool
}

// ManagePageView drives the machine management screen.
type ManagePageView struct {
	// Available is false when the privileged helper is not installed, which is
	// the whole feature missing rather than one action failing.
	Available bool
	// Pending names an action already on its way, so the screen says so
	// instead of inviting a second press into the void.
	Pending string
	Actions []ManageActionRow

	// The rest is context for deciding whether now is a good moment: what the
	// machine is doing and what would be interrupted.
	Host      SystemView
	Jellyfin  ManageServiceRow
	Torrents  ManageServiceRow
	Notice    string
	NoticeErr bool

	// Warnings names what is in flight right now. Checked is false when none
	// of it could be consulted, which is not the same as all clear.
	Warnings []string
	Checked  bool

	// Remote is true when this page was reached over the public address rather
	// than from the LAN. Read from the Host header, so it is a hint about
	// where the person is, never a permission check.
	Remote bool
}

// ManageServiceRow is what can honestly be said about a neighbouring service:
// whether Holocron can currently talk to it. That is not the same as systemd
// thinking it is running, and the screen does not claim otherwise.
type ManageServiceRow struct {
	Name       string
	Configured bool
	Reachable  bool
	Detail     string
}
