package templates

// DiskPageView is the disk page: a card per disk, what needs attention, and
// the selected disk's scan and drill-down.
type DiskPageView struct {
	Cards    []DiskCard
	Alerts   []DiskAlert
	Selected *DiskDetail
	Empty    bool
}

// DiskCard is one disk at a glance. Everything on it is read without waking
// the disk: statfs, the mount table, and the SMART file the server writes.
type DiskCard struct {
	Name, Href  string
	Active      bool
	Model, Role string
	StatOK      bool
	UsedPercent int
	UsedHuman   string
	TotalHuman  string
	FreeHuman   string
	// Checked says the mount was verified; Mounted is only meaningful then.
	Checked, Mounted bool
	Asleep           bool
	// Temp is the temperature; TempAge says when it was read, when that was
	// not just now (the disk sleeps and keeps its last reading).
	Temp, TempAge string
	// Health is a word ("sano", "falla", "sin datos de salud"); HealthTone
	// "ok", "bad" or "none".
	Health, HealthTone string
	// Realloc is "217 reasignados" with its age when old; ReallocUp marks a
	// rise over the first count seen.
	Realloc   string
	ReallocUp bool
	Libraries string // "Películas", "Series · Museum · Anime"
}

// DiskAlert is something about the disks worth acting on, with the one
// action that settles it when there is one.
type DiskAlert struct {
	Text   string
	Detail string
	// Bad is red (a disk gone); otherwise the lilac of something to look at.
	Bad bool
	// Action posts Field=Value to settle the alert, with Button as label.
	Action, Field, Value, Button string
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
	// NoAccess are folders the scan could not read (lost+found is root's),
	// listed apart because they are not a failure.
	NoAccess []string
	// Hidden is space in use that no visible folder accounts for, when it is
	// too large to be the filesystem's own bookkeeping: on Disco4, the old
	// copy of Películas under the bind mount.
	Hidden string
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
	// Foreign is a folder that is another disk mounted here; ForeignName and
	// ForeignHref lead to that disk instead of measuring it as this one.
	Foreign     bool
	ForeignName string
	ForeignHref string
}
