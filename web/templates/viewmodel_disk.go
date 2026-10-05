package templates

// DiskPageView is the disk detail page: a nav of folders and the selected one.
type DiskPageView struct {
	Nav      []DiskNavItem
	Selected *DiskDetail
	Empty    bool
}

// DiskNavItem is a folder tab on the disk page.
type DiskNavItem struct {
	Label  string
	Href   string
	Active bool
}

// DiskDetail is the selected folder's scan detail.
type DiskDetail struct {
	FolderID    string // used in the scan form and status fragment
	Label       string
	Path        string
	Scanning    bool
	HasResult   bool
	ScannedAt   string
	UsedPercent int
	UsedHuman   string
	TotalHuman  string
	FreeHuman   string
	Top         []DiskTopRow
	Errors      []string
}

// DiskTopRow is one of the largest folders in a scan. BrowseHref drills in.
type DiskTopRow struct {
	Name       string
	BrowseHref string
	BytesHuman string
	Percent    int
}

// ScanStatusView drives the scan progress fragment (HTMX polling target).
type ScanStatusView struct {
	Running    bool
	Error      string
	Summary    string
	StatusHref string // poll target while running
	ReloadHref string // detail-section reload when done
}

// BrowseView is one level of the drill-down.
type BrowseView struct {
	Path       string
	TotalHuman string
	HasParent  bool
	ParentHref string
	Entries    []BrowseRow
}

// BrowseRow is one child entry in the drill-down. Percent is the size relative
// to the largest entry in the level (0..100), for the mini proportion bar.
type BrowseRow struct {
	Name       string
	BrowseHref string // for directories
	BytesHuman string
	Percent    int
	IsDir      bool
}
