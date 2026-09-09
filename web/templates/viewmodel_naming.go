package templates

// NamingCardView drives the dashboard naming widget.
type NamingCardView struct {
	HasMediaFolders bool
	Count           int
}

// NamingPageView drives the naming detail page.
type NamingPageView struct {
	HasMediaFolders bool
	Count           int
	Issues          []NamingIssueRow
	// Ignored is what the user has told Holocron to leave alone. Shown rather
	// than kept quietly in the database: an ignore list nobody can see is a
	// bug that looks like a feature.
	Ignored []NamingIgnoredRow
	Notice  string
}

// NamingIgnoredRow is one folder being left alone.
type NamingIgnoredRow struct {
	Path string
	Name string
}

// NamingIssueRow is one folder that breaks the convention.
type NamingIssueRow struct {
	Type     string
	Found    string
	Expected string
	Path     string
}
