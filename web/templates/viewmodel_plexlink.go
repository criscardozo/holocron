package templates

// PlexLinkView drives the device-link fragment in Ajustes: it either offers to
// start, shows the code while waiting for authorisation at plex.tv, or lists
// the discovered servers to pick from.
type PlexLinkView struct {
	Pending bool
	Linked  bool
	Code    string
	AuthURL string
	Servers []PlexServerOption
	Error   string
}

// PlexServerOption is one server discovered for the account.
type PlexServerOption struct {
	Name    string
	BaseURL string
}
