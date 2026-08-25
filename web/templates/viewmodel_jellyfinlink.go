package templates

// JellyfinLinkView drives the Quick Connect fragment in Ajustes: it either
// offers to start, shows the code while waiting for approval, or reports who
// authorised.
type JellyfinLinkView struct {
	Pending bool
	Linked  bool
	Code    string
	User    string
	// Admin is false when the linked account cannot ask Jellyfin to write
	// metadata, which is worth saying before the user presses that button.
	Admin bool
	Error string
}
