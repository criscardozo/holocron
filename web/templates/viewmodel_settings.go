package templates

// SettingsView is the settings page: the list of watched folders and a form to
// add more.
type SettingsView struct {
	Folders      []SettingsFolderRow
	Purposes     []string
	Notice       string
	PlexURL      string
	PlexTokenSet bool
	OpenSubsUser string
	OpenSubsSet  bool
	QbitURL      string
	QbitUser     string
	QbitSet      bool
}

// SettingsFolderRow is one configured watched folder.
type SettingsFolderRow struct {
	ID      int64
	Label   string
	Path    string
	Purpose string
}
