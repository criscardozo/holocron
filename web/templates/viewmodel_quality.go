package templates

// QualityCount is one category of the quality report: how many items fall in
// it and where its list lives.
type QualityCount struct {
	Key    string
	Label  string
	Count  int
	Href   string
	Active bool
}

// QualityCardView drives the dashboard quality widget. With HasReport false the
// widget invites a first audit instead of showing five zeroes, which would read
// as "the library is perfect".
type QualityCardView struct {
	Configured bool
	HasReport  bool
	Scanning   bool
	Total      int
	Counts     []QualityCount
}

// QualityPageView drives the quality panel. Selected is the category being
// listed; the rest are tabs.
type QualityPageView struct {
	Configured  bool
	Scanning    bool
	HasReport   bool
	GeneratedAt string
	Scanned     int
	Total       int
	// Admin reports whether the linked Jellyfin account may ask the server to
	// re-read metadata. Without it the refresh action is not offered at all.
	Admin    bool
	Counts   []QualityCount
	Selected *QualityGroupView
}

// QualityGroupView is one category's list.
type QualityGroupView struct {
	Key   string
	Label string
	Hint  string
	Count int
	// Truncated reports that the list shows fewer rows than Count.
	Truncated   bool
	Refreshable bool
	Rows        []QualityRow
}

// QualityRow is one finding.
type QualityRow struct {
	ItemID string
	Title  string
	Detail string
	Path   string
	Kind   string
}

// QualityRefreshView is the fragment that replaces a row's action cell after a
// refresh was asked for.
type QualityRefreshView struct {
	Message string
	Error   bool
}
