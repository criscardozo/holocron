package templates

// MediaCardView drives the dashboard media widget.
type MediaCardView struct {
	Configured  bool
	Total       int
	WithNFO     int
	WithoutSubs int
}

// MediaPageView drives the media detail page. Truncated reports that the
// inventory table shows only the first page of a larger library.
type MediaPageView struct {
	Configured    bool
	Syncing       bool
	GeneratingNFO bool
	Total         int
	WithNFO       int
	WithoutSubs   int
	Truncated     bool
	Items         []MediaItemRow
}

// MediaItemRow is one inventory row.
type MediaItemRow struct {
	Title     string
	Year      int
	Type      string
	Path      string
	HasNFO    bool
	HasSubsES bool
}

// JobStatusView is a generic background-job progress fragment. While running it
// polls StatusHref; when done it reloads a section of the page.
type JobStatusView struct {
	Running      bool
	Error        string
	Summary      string
	Label        string
	StatusHref   string
	ReloadHref   string
	ReloadSelect string
	ReloadTarget string
}
