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
}
