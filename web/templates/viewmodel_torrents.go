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

// TorrentsPageView drives the torrents page.
type TorrentsPageView struct {
	Configured bool
	Err        string
	Rows       []TorrentRow
}

// TorrentRow is one torrent in the table.
type TorrentRow struct {
	Hash      string
	Name      string
	State     string
	Percent   int
	SizeHuman string
	DlHuman   string
	UpHuman   string
	Seeds     int
	Leechs    int
	Paused    bool
}
