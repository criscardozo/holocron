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
}

// SettingsFolderRow is one configured watched folder.
type SettingsFolderRow struct {
	ID      int64
	Label   string
	Path    string
	Purpose string
}
