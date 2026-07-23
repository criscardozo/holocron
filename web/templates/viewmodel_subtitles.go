package templates

// SubtitlesCardView drives the dashboard subtitles widget.
type SubtitlesCardView struct {
	Configured bool
	Missing    int
}

// SubtitlesPageView drives the subtitles page.
type SubtitlesPageView struct {
	Configured bool
	Rows       []SubtitleMissingRow
}

// SubtitleMissingRow is one media item lacking a Spanish subtitle.
type SubtitleMissingRow struct {
	Title      string
	Year       int
	Type       string
	ResultsID  string // DOM id of this row's results container
	SearchHref string // GET endpoint that returns search results for this item
}

// SubtitleSearchView is the search-results fragment for one item.
type SubtitleSearchView struct {
	Path    string
	None    bool
	Results []SubtitleResultRow
}

// SubtitleResultRow is one downloadable subtitle option.
type SubtitleResultRow struct {
	FileID   string
	FileName string
	Release  string
	Language string
}
