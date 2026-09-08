package templates

// TrailerFilmRow is one film with no trailer.
type TrailerFilmRow struct {
	// Key is the absolute folder, and what the fetch form sends back. It is
	// only ever accepted if it matches a folder the scan itself found.
	Key    string
	Folder string
	Title  string
	Year   string
	// NoYear marks the ones whose folder carries no year, which makes the
	// search vaguer and the result worth checking.
	NoYear bool
}

// TrailerResultRow is what happened to one film in the last fetch.
type TrailerResultRow struct {
	Folder  string
	Trailer string
	Reason  string
	Err     string
}

// TrailersPageView drives the trailers screen.
type TrailersPageView struct {
	HasMediaFolders bool
	Running         bool
	Scanned         bool

	// ToolPath is empty when yt-dlp is not installed, which is a normal state
	// and gets its own explanation rather than a generic failure.
	ToolPath    string
	ToolVersion string
	ToolStale   bool

	Films   []TrailerFilmRow
	Results []TrailerResultRow

	Notice    string
	NoticeErr bool
}

// ToolMissing reports whether yt-dlp is absent.
func (v TrailersPageView) ToolMissing() bool { return v.ToolPath == "" }
