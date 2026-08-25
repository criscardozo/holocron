package templates

// TorrentsCardView drives the dashboard torrents widget.
type TorrentsCardView struct {
	Configured bool
	Err        bool
	Total      int
	Active     int
	DlHuman    string
	UpHuman    string
}

// TorrentsPageView drives the torrents page. Categories are the ones defined in
// qBittorrent, offered when adding a magnet.
type TorrentsPageView struct {
	Configured bool
	Err        string
	Categories []string
	// RefreshEvery is the HTMX polling interval for the table. It stretches
	// when nothing is transferring: a Pi with a busy disk should not be asked
	// for an unchanged list every few seconds. Deciding it here keeps the page
	// free of JavaScript, which a visibility-based filter would need (and the
	// strict CSP forbids eval).
	RefreshEvery string
	Rows         []TorrentRow
}

// TorrentRow is one torrent in the table.
type TorrentRow struct {
	Hash      string
	Name      string
	State     string
	Category  string
	Percent   int
	SizeHuman string
	DlHuman   string
	UpHuman   string
	Seeds     int
	Leechs    int
	Paused    bool
}
