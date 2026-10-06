package templates

// AppLink is one app on the start page: a large card for the two the house
// uses every day, a small chip for the machinery behind them.
type AppLink struct {
	Name     string `json:"name"`
	Short    string `json:"short"`
	URL      string `json:"url"`
	Logo     string `json:"logo"` // path on this server
	Featured bool   `json:"featured"`
}

// StackView is the Stack page: every app, what it is for, and how to reach it.
type StackView struct {
	Featured []StackApp
	Others   []StackApp
	// Direct says the direct links are there, so the page can explain them.
	Direct bool
	// Watched says the server watches the units, so a missing state means
	// "not watched" rather than "unknown".
	Watched bool
}

// StackApp is one app on the Stack page.
type StackApp struct {
	Name, Purpose string
	URL, Host     string // by name, and the host part to show
	Direct        string // http://<lan>:<port>/, or ""
	DirectLabel   string // <lan>:<port>
	Logo          string
	Version       string
	// State is systemd's word in Spanish ("corriendo", "caído"); OK whether
	// that is good. Empty when the unit is not watched.
	State string
	OK    bool
}
