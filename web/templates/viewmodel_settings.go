package templates

// SettingsView is the settings page: the list of watched folders and a form to
// add more.
type SettingsView struct {
	Folders      []SettingsFolderRow
	Purposes     []string
	Notice       string
	JellyfinURL  string
	OpenSubsUser string
	OpenSubsSet  bool
	QbitURL      string
	QbitUser     string
	QbitSet      bool
	// APITokenSet reports whether a JSON API token exists. The token itself is
	// never shown again: only its digest is stored.
	APITokenSet bool
	// JellyfinLink drives the Quick Connect fragment.
	JellyfinLink JellyfinLinkView
	// Updates drives the update panel.
	Updates UpdatesView

	// One per credential card, so each says plainly whether it is set up.
	Jellyfin SettingsCred
	OpenSubs SettingsCred
	Qbit     SettingsCred
}

// SettingsFolderRow is one configured watched folder.
type SettingsFolderRow struct {
	ID      int64
	Label   string
	Path    string
	Purpose string
}

// SettingsFact is one thing worth showing about a configured service. Never a
// secret: the point is to confirm what is stored, not to reveal it.
type SettingsFact struct {
	Label string
	Value string
}

// SettingsCred is a credential card's state.
//
// The card used to look identical whether or not anything was saved — empty
// inputs either way, with a small "guardado" tick beside one label. That reads
// as "not set up yet", so the honest thing is to stop offering a form for
// something that is already done and say so instead.
type SettingsCred struct {
	Configured bool
	// Facts are shown when configured, in place of the form.
	Facts []SettingsFact
	// ClearHref forgets the credentials so the form comes back. There is no
	// edit-in-place: half-changing a set of credentials is how you end up with
	// a URL from one server and a password from another.
	ClearHref string
	Confirm   string
	// StatusHref lazily loads a live check. Stored is not the same as working,
	// and the difference is the whole question someone opens this page with —
	// but probing three services while the page waits would make it slow, so
	// it arrives after the render.
	StatusHref string
}
