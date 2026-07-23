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
}

// NamingIssueRow is one folder that breaks the convention.
type NamingIssueRow struct {
	Type     string
	Found    string
	Expected string
	Path     string
}
