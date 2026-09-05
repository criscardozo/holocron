package templates

// UpdatesView drives the update panel in Ajustes.
type UpdatesView struct {
	Current   string
	Latest    string
	Notes     string
	URL       string
	Available bool
	// Checkable is false for a dev build, which has no tag to compare.
	Checkable bool
	// Installable is false when the privileged helper is missing; the panel
	// then shows the manual command instead of a button.
	Installable bool
	// Requested is true right after asking for an update, while the service
	// is being replaced and restarted.
	Requested string
	Error     string
	CheckedAt string
	// Checked is false until GitHub has actually been consulted. Without it the
	// panel said "estás al día" while never having looked, which is how a Pi
	// sat three releases behind — including the one that fixed its Jellyfin
	// connection — with the page reporting it was current.
	Checked bool
}
